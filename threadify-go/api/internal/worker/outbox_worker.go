package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/utils"
	sharedauth "threadify-go/shared/auth"

	"github.com/nats-io/nats.go"
)

const (
	batchSize    = 10
	interval     = 5 * time.Second
	backoffDelay = 10 * time.Second
)

type OutboxWorker struct {
	outboxRepo    *repository.OutboxRepository
	userRepo      *repository.UserRepository
	authClient    sharedauth.AuthClient
	emailSvc      *service.EmailService
	encryptionKey []byte
	trigger       chan struct{}
}

func NewOutboxWorker(
	outboxRepo *repository.OutboxRepository,
	userRepo *repository.UserRepository,
	authClient sharedauth.AuthClient,
	emailSvc *service.EmailService,
	encryptionKey string,
) *OutboxWorker {
	return &OutboxWorker{
		outboxRepo:    outboxRepo,
		userRepo:      userRepo,
		authClient:    authClient,
		emailSvc:      emailSvc,
		encryptionKey: []byte(encryptionKey),
		trigger:       make(chan struct{}, 1),
	}
}

func (w *OutboxWorker) Trigger() {
	select {
	case w.trigger <- struct{}{}:
	default:
	}
}

func (w *OutboxWorker) Run(ctx context.Context, js nats.JetStreamContext) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sub, err := js.PullSubscribe("outbox.trigger", "outbox-worker")
	if err != nil {
		log.Printf("outbox: failed to subscribe to NATS trigger: %v", err)
	} else {
		defer sub.Unsubscribe()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processEvents(ctx)
		case <-w.trigger:
			w.processEvents(ctx)
		default:
			if sub != nil {
				msgs, err := sub.Fetch(1, nats.MaxWait(100*time.Millisecond))
				if err == nil && len(msgs) > 0 {
					w.processEvents(ctx)
					msgs[0].Ack()
				}
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func (w *OutboxWorker) processEvents(ctx context.Context) {
	events, err := w.outboxRepo.FetchPendingDue(batchSize)
	if err != nil {
		log.Printf("outbox: failed to fetch pending events: %v", err)
		return
	}

	for _, event := range events {
		if err := w.handleEvent(ctx, event); err != nil {
			log.Printf("outbox: event %s (%s) attempt %d failed: %v",
				event.ID, event.Type, event.RetryCount+1, err)

			backoff := time.Duration(math.Pow(2, float64(event.RetryCount))) * backoffDelay
			if err := w.outboxRepo.MarkFailedWithRetry(event.ID, err.Error(), time.Now().Add(backoff)); err != nil {
				log.Printf("outbox: failed to mark event %s for retry: %v", event.ID, err)
			}
			continue
		}

		if err := w.outboxRepo.MarkDone(event.ID); err != nil {
			log.Printf("outbox: failed to mark event %s as done: %v", event.ID, err)
		}
	}
}

func (w *OutboxWorker) handleEvent(ctx context.Context, event *models.OutboxEvent) error {
	plaintext, err := utils.Decrypt(event.Payload, w.encryptionKey)
	if err != nil {
		return fmt.Errorf("decrypt payload: %w", err)
	}
	defer func() {
		for i := range plaintext {
			plaintext[i] = 0
		}
	}()

	var data map[string]string
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	switch event.Type {
	case models.EventTypeRegisterAuthUser:
		return w.handleRegisterAuthUser(ctx, event, data)
	case models.EventTypeSendVerificationEmail:
		return w.handleSendVerificationEmail(ctx, data)
	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}

func (w *OutboxWorker) handleRegisterAuthUser(ctx context.Context, event *models.OutboxEvent, data map[string]string) error {
	userID, err := getField(data, "user_id")
	if err != nil {
		return err
	}
	email, err := getField(data, "email")
	if err != nil {
		return err
	}
	password, err := getField(data, "password")
	if err != nil {
		return err
	}
	fullName, err := getField(data, "full_name")
	if err != nil {
		return err
	}
	companyID, err := getField(data, "company_id")
	if err != nil {
		return err
	}

	user, err := w.userRepo.FindByID(userID)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %s not found", userID)
	}

	if user.AuthUserID == nil || *user.AuthUserID == "" {
		authUserID, err := w.authClient.RegisterUser(ctx, email, password, fullName, userID, companyID)
		if err != nil {
			return fmt.Errorf("register auth user: %w", err)
		}
		if err := w.userRepo.UpdateAuthUserID(userID, authUserID); err != nil {
			return fmt.Errorf("update auth user ID: %w", err)
		}
	}

	payload, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return fmt.Errorf("marshal email event payload: %w", err)
	}
	encrypted, err := utils.Encrypt(payload, w.encryptionKey)
	for i := range payload {
		payload[i] = 0
	}
	if err != nil {
		return fmt.Errorf("encrypt email event payload: %w", err)
	}

	return w.outboxRepo.CreateTx(nil, &models.OutboxEvent{
		ID:         utils.GenerateID(),
		Type:       models.EventTypeSendVerificationEmail,
		Payload:    encrypted,
		Status:     models.OutboxStatusPending,
		MaxRetries: 5,
		NextRunAt:  time.Now(),
	})
}

func (w *OutboxWorker) handleSendVerificationEmail(ctx context.Context, data map[string]string) error {
	email, err := getField(data, "email")
	if err != nil {
		return err
	}

	if u, err := w.userRepo.FindByEmail(email); err == nil && u != nil && u.EmailVerified {
		return nil
	}

	token, err := w.authClient.GenerateSignupLink(ctx, email)
	if err != nil {
		if errors.Is(err, sharedauth.ErrAuthUserAlreadyExists) {
			log.Printf("outbox: skipping email for %s - user already exists/confirmed in auth provider", email)
			return nil
		}
		return fmt.Errorf("generate signup link: %w", err)
	}

	if err := w.emailSvc.SendVerificationEmail(ctx, email, token); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}

	return nil
}

func getField(data map[string]string, key string) (string, error) {
	v, ok := data[key]
	if !ok || v == "" {
		return "", fmt.Errorf("payload missing required field: %s", key)
	}
	return v, nil
}
