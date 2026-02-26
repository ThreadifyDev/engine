package service

import (
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type NatsOutboxTrigger struct {
	js      nats.JetStreamContext
	subject string
	logger  *zap.Logger
}

func NewNatsOutboxTrigger(js nats.JetStreamContext, subject string, logger *zap.Logger) *NatsOutboxTrigger {
	return &NatsOutboxTrigger{
		js:      js,
		subject: subject,
		logger:  logger,
	}
}

func (t *NatsOutboxTrigger) Trigger() {
	_, err := t.js.Publish(t.subject, []byte("process"))
	if err != nil {
		t.logger.Error("nats_trigger: failed to publish outbox trigger",
			zap.String("subject", t.subject),
			zap.Error(err),
		)
	}
}
