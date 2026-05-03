package service

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

type NatsOutboxTrigger struct {
	js      jetstream.JetStream
	subject string
	logger  *zap.Logger
}

func NewNatsOutboxTrigger(
	js jetstream.JetStream,
	subject string,
	logger *zap.Logger,
) *NatsOutboxTrigger {
	return &NatsOutboxTrigger{
		js:      js,
		subject: subject,
		logger:  logger,
	}
}

func (t *NatsOutboxTrigger) Trigger() {
	_, err := t.js.Publish(context.Background(), t.subject, []byte("process"))
	if err != nil {
		t.logger.Warn("nats_trigger: failed to publish outbox trigger",
			zap.String("subject", t.subject),
			zap.Error(err),
		)
	}

	t.logger.Info("nats_trigger: outbox trigger published successfully",
		zap.String("subject", t.subject),
	)
}
