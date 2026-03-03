package worker

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/utils"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/nats"

	natsio "github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

const (
	batchSize    = 10
	interval     = 5 * time.Second
	backoffDelay = 10 * time.Second
)

type OutboxWorker struct {
	outboxRepo    *repository.OutboxRepository
	userRepo      *repository.UserRepository
	companyRepo   *repository.CompanyRepository
	authClient    sharedauth.AuthClient
	emailSvc      *service.EmailService
	encryptionKey []byte
	trigger       chan struct{}
	logger        *zap.Logger
}

func NewOutboxWorker(
	outboxRepo *repository.OutboxRepository,
	userRepo *repository.UserRepository,
	companyRepo *repository.CompanyRepository,
	authClient sharedauth.AuthClient,
	emailSvc *service.EmailService,
	encryptionKey string,
	logger *zap.Logger,
) *OutboxWorker {
	key, err := hex.DecodeString(encryptionKey)
	if err != nil {
		logger.Error("failed to decode outbox encryption key", zap.Error(err))
		// Fallback to raw bytes if hex decoding fails.
		key = []byte(encryptionKey)
	}

	return &OutboxWorker{
		outboxRepo:    outboxRepo,
		userRepo:      userRepo,
		companyRepo:   companyRepo,
		authClient:    authClient,
		emailSvc:      emailSvc,
		encryptionKey: key,
		trigger:       make(chan struct{}, 1),
		logger:        logger,
	}
}

func (w *OutboxWorker) Trigger() {
	select {
	case w.trigger <- struct{}{}:
	default:
	}
}

func (w *OutboxWorker) Run(ctx context.Context, js natsio.JetStreamContext) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sub, err := js.PullSubscribe(nats.SubjectOutboxTrigger, "outbox-worker")
	if err != nil {
		w.logger.Error("outbox: failed to subscribe to NATS trigger", zap.Error(err))
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
			if sub != nil {
				msgs, err := sub.Fetch(1, natsio.MaxWait(100*time.Millisecond))
				if err == nil && len(msgs) > 0 {
					msgs[0].Ack()
				}
			}
			w.processEvents(ctx)
		}
	}
}

func (w *OutboxWorker) processEvents(ctx context.Context) {
	events, err := w.outboxRepo.FetchPendingDue(batchSize)
	if err != nil {
		w.logger.Error("outbox: failed to fetch pending events", zap.Error(err))
		return
	}
	if len(events) == 0 {
		return
	}
	w.logger.Debug("outbox: processing events", zap.Int("count", len(events)))

	for _, event := range events {
		if err := w.handleEvent(ctx, event); err != nil {
			w.logger.Error("outbox: event processing failed",
				zap.String("event_id", event.ID),
				zap.String("event_type", event.Type),
				zap.String("reference_id", event.ReferenceID),
				zap.Int("attempt", event.RetryCount+1),
				zap.Error(err),
			)
			retryExp := event.RetryCount
			if retryExp > 20 {
				retryExp = 20
			}
			backoff := time.Duration(math.Pow(2, float64(retryExp))) * backoffDelay
			if err := w.outboxRepo.MarkFailedWithRetry(event.ID, err.Error(), time.Now().Add(backoff)); err != nil {
				w.logger.Error("outbox: failed to mark event for retry",
					zap.String("event_id", event.ID),
					zap.Error(err),
				)
			}
			continue
		}

		if err := w.outboxRepo.MarkDone(event.ID); err != nil {
			w.logger.Error("outbox: failed to mark event as done",
				zap.String("event_id", event.ID),
				zap.Error(err),
			)
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
		w.logger.Debug("outbox: dispatching register-auth-user",
			zap.String("event_id", event.ID),
			zap.String("reference_id", event.ReferenceID),
		)
		return w.handleRegisterAuthUser(ctx, event, data)
	case models.EventTypeSendVerificationEmail:
		w.logger.Debug("outbox: dispatching send-verification-email",
			zap.String("event_id", event.ID),
			zap.String("reference_id", event.ReferenceID),
		)
		return w.handleSendVerificationEmail(ctx, data)
	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}

func (w *OutboxWorker) handleRegisterAuthUser(ctx context.Context, _ *models.OutboxEvent, data map[string]string) error {
	fields, err := getFields(data, "user_id", "email", "password", "company_id")
	if err != nil {
		return err
	}
	userID, email, password, companyID := fields[0], fields[1], fields[2], fields[3]

	var fullName string
	if val, ok := data["full_name"]; ok {
		fullName = val
	}

	user, err := w.userRepo.FindByID(userID)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user %s not found", userID)
	}

	if user.AuthUserID == nil || *user.AuthUserID == "" {
		w.logger.Debug("outbox: registering user in auth provider",
			zap.String("user_id", userID),
		)
		authUserID, err := w.resolveAuthUserID(ctx, email, password, fullName, userID, companyID)
		if err != nil {
			return err
		}
		if authUserID == "" {
			w.logger.Warn("outbox: discarding event — permanent auth registration failure",
				zap.String("user_id", userID),
			)
			return nil
		}

		if err := w.userRepo.UpdateAuthUserID(userID, authUserID); err != nil {
			return fmt.Errorf("update auth user ID: %w", err)
		}

		w.logger.Info("outbox: user registered in auth provider",
			zap.String("user_id", userID),
			zap.String("auth_user_id", authUserID),
		)
	}

	alreadyQueued, err := w.outboxRepo.ExistsByReference(models.EventTypeSendVerificationEmail, userID)
	if err != nil {
		return fmt.Errorf("check existing verification email event: %w", err)
	}
	if alreadyQueued {
		w.logger.Debug("outbox: verification email already queued, skipping",
			zap.String("user_id", userID),
		)
		return nil
	}

	return w.queueVerificationEmail(userID, email)
}

func (w *OutboxWorker) queueVerificationEmail(userID, email string) error {
	payload, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		return fmt.Errorf("marshal email event payload: %w", err)
	}
	encrypted, err := utils.Encrypt(payload, w.encryptionKey)
	if err != nil {
		for i := range payload {
			payload[i] = 0
		}
		return fmt.Errorf("encrypt email event payload: %w", err)
	}
	for i := range payload {
		payload[i] = 0
	}

	if err := w.outboxRepo.Create(&models.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        models.EventTypeSendVerificationEmail,
		Payload:     encrypted,
		Status:      models.OutboxStatusPending,
		MaxRetries:  models.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: userID,
	}); err != nil {
		return fmt.Errorf("create verification email event: %w", err)
	}

	w.logger.Info("outbox: queued verification email",
		zap.String("user_id", userID),
	)
	return nil
}

func (w *OutboxWorker) resolveAuthUserID(ctx context.Context, email, password, fullName, userID, companyID string) (string, error) {
	authUserID, err := w.authClient.RegisterUser(ctx, email, password, fullName, userID, companyID)
	if err == nil {
		return authUserID, nil
	}

	switch {
	case errors.Is(err, sharedauth.ErrAuthUserAlreadyExists):
		recovered, fetchErr := w.authClient.FindUserIDByEmail(ctx, email)
		if fetchErr != nil {
			return "", fmt.Errorf("recover existing auth user: %w", fetchErr)
		}
		w.logger.Info("outbox: recovered existing auth user",
			zap.String("auth_user_id", recovered),
		)
		return recovered, nil

	case errors.Is(err, sharedauth.ErrAuthInvalidEmail):
		w.logger.Error("outbox: permanent registration failure, cleaning up",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		if cleanupErr := w.cleanupUser(ctx, userID, companyID); cleanupErr != nil {
			return "", cleanupErr
		}
		return "", nil

	default:
		return "", fmt.Errorf("register auth user: %w", err)
	}
}

func (w *OutboxWorker) cleanupUser(ctx context.Context, userID, companyID string) error {
	tx, err := w.userRepo.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cleanup transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := w.userRepo.DeleteTx(tx, userID); err != nil {
		return fmt.Errorf("cleanup delete user: %w", err)
	}
	if err := w.companyRepo.DeleteTx(tx, companyID); err != nil {
		return fmt.Errorf("cleanup delete company: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cleanup transaction: %w", err)
	}

	w.logger.Info("outbox: orphaned user cleaned up",
		zap.String("user_id", userID),
		zap.String("company_id", companyID),
	)
	return nil
}

func (w *OutboxWorker) handleSendVerificationEmail(ctx context.Context, data map[string]string) error {
	email, err := getField(data, "email")
	if err != nil {
		return err
	}

	if u, err := w.userRepo.FindByEmail(email); err == nil && u != nil && u.EmailVerified {
		w.logger.Debug("outbox: user already verified, skipping email")
		return nil
	}

	w.logger.Debug("outbox: generating verification OTP")
	token, err := w.authClient.GenerateLoginOTP(ctx, email)
	if err != nil {
		return fmt.Errorf("generate verification otp: %w", err)
	}

	if err := w.emailSvc.SendVerificationEmail(ctx, email, token); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}

	w.logger.Info("outbox: verification email sent")
	return nil
}

func getFields(data map[string]string, keys ...string) ([]string, error) {
	out := make([]string, len(keys))
	for i, k := range keys {
		v, err := getField(data, k)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func getField(data map[string]string, key string) (string, error) {
	v, ok := data[key]
	if !ok || v == "" {
		return "", fmt.Errorf("payload missing required field: %s", key)
	}
	return v, nil
}
