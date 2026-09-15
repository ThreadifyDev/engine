// Package natsfixture connects tests only to an explicitly supplied disposable broker.
package natsfixture

import (
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func New(t testing.TB) jetstream.JetStream {
	t.Helper()
	endpoint := os.Getenv("THREADIFY_REGISTRY_TEST_NATS_URL")
	if endpoint == "" {
		t.Skip("set THREADIFY_REGISTRY_TEST_NATS_URL to a disposable JetStream server")
	}
	nc, err := nats.Connect(endpoint, nats.Timeout(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	return js
}
