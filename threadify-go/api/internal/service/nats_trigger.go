package service

import (
	"log"

	"github.com/nats-io/nats.go"
)

type NatsOutboxTrigger struct {
	js      nats.JetStreamContext
	subject string
}

func NewNatsOutboxTrigger(js nats.JetStreamContext, subject string) *NatsOutboxTrigger {
	return &NatsOutboxTrigger{
		js:      js,
		subject: subject,
	}
}

func (t *NatsOutboxTrigger) Trigger() {
	_, err := t.js.Publish(t.subject, []byte("process"))
	if err != nil {
		log.Printf("nats_trigger: failed to publish outbox trigger: %v", err)
	}
}
