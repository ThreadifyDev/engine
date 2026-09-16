package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"threadify-go/api/internal/service"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/management/domain"
	"threadify-go/shared/management/utils"
	"threadify-go/shared/nats"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

const (
	batchSize    = 10
	interval     = 5 * time.Second
	backoffDelay = 10 * time.Second
)

type OutboxWorker struct {
	pool          *pgxpool.Pool
	outboxRepo    domain.OutboxRepository
	userRepo      domain.UserRepository
	companyRepo   domain.CompanyRepository
	authClient    sharedauth.AuthClient
	emailSvc      service.EmailService
	encryptionKey []byte
	trigger       chan struct{}
	logger        *zap.Logger
}

func NewOutboxWorker(
	pool *pgxpool.Pool,
	outboxRepo domain.OutboxRepository,
	userRepo domain.UserRepository,
	companyRepo domain.CompanyRepository,
	authClient sharedauth.AuthClient,
	emailSvc service.EmailService,
	encryptionKey []byte,
	logger *zap.Logger,
) *OutboxWorker {

	return &OutboxWorker{
		pool:          pool,
		outboxRepo:    outboxRepo,
		userRepo:      userRepo,
		companyRepo:   companyRepo,
		authClient:    authClient,
		emailSvc:      emailSvc,
		encryptionKey: encryptionKey,
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

func (w *OutboxWorker) Run(ctx context.Context, js jetstream.JetStream) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	consumerConfig := jetstream.ConsumerConfig{
		Durable:       "outbox-worker",
		FilterSubject: nats.SubjectOutboxTrigger,
		AckPolicy:     jetstream.AckExplicitPolicy,
	}

	cons, err := js.CreateOrUpdateConsumer(ctx, nats.StreamOutboxTriggers, consumerConfig)
	if err != nil {
		w.logger.Error("outbox: failed to subscribe to NATS trigger", zap.Error(err))
	}

	var consumeCtx jetstream.ConsumeContext
	if cons != nil {
		consumeCtx, err = cons.Consume(func(msg jetstream.Msg) {
			_ = msg.Ack()
			w.Trigger()
		}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, consumeErr error) {
			if consumeErr != nil {
				w.logger.Warn("outbox: NATS trigger consumer error", zap.Error(consumeErr))
			}
		}))
		if err != nil {
			w.logger.Warn("outbox: failed to start NATS trigger consumer; falling back to polling", zap.Error(err))
		}
	}
	if consumeCtx != nil {
		defer consumeCtx.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processEvents(ctx)
		case <-w.trigger:
			w.processEvents(ctx)
		}
	}
}

func (w *OutboxWorker) processEvents(ctx context.Context) {
	events, err := w.outboxRepo.FetchPendingDue(ctx, batchSize)
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
			if err := w.outboxRepo.MarkFailedWithRetry(ctx, event.ID, err.Error(), time.Now().Add(backoff)); err != nil {
				w.logger.Error("outbox: failed to mark event for retry",
					zap.String("event_id", event.ID),
					zap.Error(err),
				)
			}
			continue
		}

		if err := w.outboxRepo.MarkDone(ctx, event.ID); err != nil {
			w.logger.Error("outbox: failed to mark event as done",
				zap.String("event_id", event.ID),
				zap.Error(err),
			)
		}
	}
}

func (w *OutboxWorker) handleEvent(ctx context.Context, event *domain.OutboxEvent) error {
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
	case domain.EventTypeRegisterAuthUser:
		w.logger.Debug("outbox: dispatching register-auth-user",
			zap.String("event_id", event.ID),
			zap.String("reference_id", event.ReferenceID),
		)
		return w.handleRegisterAuthUser(ctx, event, data)
	case domain.EventTypeSendVerificationEmail:
		w.logger.Debug("outbox: dispatching send-verification-email",
			zap.String("event_id", event.ID),
			zap.String("reference_id", event.ReferenceID),
		)
		return w.handleSendVerificationEmail(ctx, data)
	case domain.EventTypeSendPasswordResetEmail:
		w.logger.Debug("outbox: dispatching send-password-reset-email",
			zap.String("event_id", event.ID),
			zap.String("reference_id", event.ReferenceID),
		)
		return w.handleSendPasswordResetEmail(ctx, data)
	case domain.EventTypeMigrateLegacyUser:
		w.logger.Debug("outbox: dispatching migrate-legacy-user",
			zap.String("event_id", event.ID),
			zap.String("reference_id", event.ReferenceID),
		)
		return w.handleMigrateLegacyUser(ctx, data)
	case domain.EventTypeSendTeamInvitation:
		w.logger.Debug("outbox: dispatching send-team-invitation",
			zap.String("event_id", event.ID),
			zap.String("reference_id", event.ReferenceID),
		)
		return w.handleSendTeamInvitation(ctx, data)
	case domain.EventTypeUpdateAuthUserEmail:
		w.logger.Debug("outbox: dispatching update-auth-user-email",
			zap.String("event_id", event.ID),
			zap.String("reference_id", event.ReferenceID),
		)
		return w.handleUpdateAuthUserEmail(ctx, data)
	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}

func (w *OutboxWorker) handleRegisterAuthUser(ctx context.Context, _ *domain.OutboxEvent, data map[string]string) error {
	fields, err := getFields(data, "user_id", "email", "password", "company_id")
	if err != nil {
		return err
	}
	userID, email, password, companyID := fields[0], fields[1], fields[2], fields[3]

	var fullName string
	if val, ok := data["full_name"]; ok {
		fullName = val
	}

	user, err := w.userRepo.FindByID(ctx, userID)
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

		if err := w.userRepo.UpdateAuthUserID(ctx, userID, authUserID); err != nil {
			return fmt.Errorf("update auth user ID: %w", err)
		}

		w.logger.Info("outbox: user registered in auth provider",
			zap.String("user_id", userID),
			zap.String("auth_user_id", authUserID),
		)
	}

	alreadyQueued, err := w.outboxRepo.ExistsByReference(ctx, domain.EventTypeSendVerificationEmail, userID)
	if err != nil {
		return fmt.Errorf("check existing verification email event: %w", err)
	}
	if alreadyQueued {
		w.logger.Debug("outbox: verification email already queued, skipping",
			zap.String("user_id", userID),
		)
		return nil
	}

	return w.queueVerificationEmail(ctx, userID, email)
}

func (w *OutboxWorker) queueVerificationEmail(ctx context.Context, userID, email string) error {
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

	if err := w.outboxRepo.Create(ctx, &domain.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        domain.EventTypeSendVerificationEmail,
		Payload:     encrypted,
		Status:      domain.OutboxStatusPending,
		MaxRetries:  domain.OutboxDefaultMaxRetries,
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
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cleanup transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := w.userRepo.DeleteTx(ctx, tx, userID); err != nil {
		return fmt.Errorf("cleanup delete user: %w", err)
	}
	if err := w.companyRepo.DeleteTx(ctx, tx, companyID); err != nil {
		return fmt.Errorf("cleanup delete company: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
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

	if u, err := w.userRepo.FindByEmail(ctx, email); err == nil && u != nil && u.EmailVerified {
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

func (w *OutboxWorker) handleSendPasswordResetEmail(ctx context.Context, data map[string]string) error {
	fields, err := getFields(data, "email", "token")
	if err != nil {
		return err
	}
	email, token := fields[0], fields[1]

	w.logger.Debug("outbox: sending password reset email for legacy user migration",
		zap.String("email", email),
	)

	if err := w.emailSvc.SendPasswordResetEmail(ctx, email, token); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}

	w.logger.Info("outbox: password reset email sent for legacy user migration",
		zap.String("email", email),
	)
	return nil
}

func (w *OutboxWorker) handleMigrateLegacyUser(ctx context.Context, data map[string]string) error {
	fields, err := getFields(data, "email", "user_id", "password", "source")
	if err != nil {
		return err
	}
	email, userID, password, source := fields[0], fields[1], fields[2], fields[3]

	w.logger.Debug("outbox: migrating legacy user to Supabase",
		zap.String("user_id", userID),
		zap.String("source", source),
	)

	user, err := w.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}

	if user.AuthUserID != nil && *user.AuthUserID != "" {
		w.logger.Info("outbox: user already migrated, skipping",
			zap.String("user_id", userID),
			zap.String("auth_user_id", *user.AuthUserID),
		)
		return nil
	}

	// Get full name
	var fullName string
	if user.FullName != nil {
		fullName = *user.FullName
	}

	// Create user in Supabase with password
	authUserID, err := w.authClient.RegisterUser(ctx, email, password, fullName, userID, user.CompanyID)
	if err != nil {
		return fmt.Errorf("register user in Supabase: %w", err)
	}

	if err := w.userRepo.UpdateAuthUserID(ctx, userID, authUserID); err != nil {
		return fmt.Errorf("update auth_user_id: %w", err)
	}

	// Clear password_hash after successful migration
	if err := w.userRepo.ClearPasswordHash(ctx, userID); err != nil {
		w.logger.Error("outbox: failed to clear password hash after migration",
			zap.Error(err),
			zap.String("user_id", userID),
		)
		// Don't fail the migration - password_hash will be cleared on next login attempt
	}

	w.logger.Debug("outbox: legacy user migration completed",
		zap.String("user_id", userID),
		zap.String("auth_user_id", authUserID),
	)

	// Send appropriate email based on source
	if source == "forgot_password" {
		// Generate password reset token for the newly created Supabase user
		resetToken, err := w.authClient.GeneratePasswordResetToken(ctx, email)
		if err != nil {
			return fmt.Errorf("generate password reset token: %w", err)
		}

		// Send password reset email
		if err := w.emailSvc.SendPasswordResetEmail(ctx, email, resetToken); err != nil {
			return fmt.Errorf("send password reset email: %w", err)
		}

		w.logger.Info("outbox: password reset email sent for migrated user",
			zap.String("user_id", userID),
			zap.String("email", email),
		)
	} else {
		// For login source, OTP verification email is queued separately
		otpCode, err := w.authClient.GenerateLoginOTP(ctx, email)
		if err != nil {
			w.logger.Error("outbox: failed to generate otp", zap.Error(err))
			return fmt.Errorf("failed to generate login verification code")
		}

		if err := w.emailSvc.SendLoginOTPEmail(ctx, email, otpCode); err != nil {
			w.logger.Error("outbox: failed to send otp email", zap.Error(err))
			return fmt.Errorf("failed to send login verification email")
		}
	}

	return nil
}

func (w *OutboxWorker) handleSendTeamInvitation(ctx context.Context, data map[string]string) error {
	fields, err := getFields(data, "email", "role", "invite_link")
	if err != nil {
		return err
	}
	email, role, inviteLink := fields[0], fields[1], fields[2]

	w.logger.Debug("outbox: sending team invitation email",
		zap.String("email", email),
		zap.String("role", role),
	)

	if err := w.emailSvc.SendTeamInvitationEmail(ctx, email, role, inviteLink); err != nil {
		return fmt.Errorf("send team invitation email: %w", err)
	}

	w.logger.Info("outbox: team invitation email sent",
		zap.String("email", email),
	)
	return nil
}

func (w *OutboxWorker) handleUpdateAuthUserEmail(ctx context.Context, data map[string]string) error {
	fields, err := getFields(data, "auth_user_id", "new_email")
	if err != nil {
		return err
	}
	authUserID, newEmail := fields[0], fields[1]

	w.logger.Debug("outbox: updating Supabase user email",
		zap.String("auth_user_id", authUserID),
		zap.String("new_email", newEmail),
	)

	if err := w.authClient.UpdateUserEmail(ctx, authUserID, newEmail); err != nil {
		return fmt.Errorf("update Supabase user email: %w", err)
	}

	w.logger.Info("outbox: Supabase user email updated",
		zap.String("auth_user_id", authUserID),
		zap.String("new_email", newEmail),
	)
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
