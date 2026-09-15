package registry

import (
	"context"
	"testing"
	"threadify-go/shared/testutil/natsfixture"
)

func enableTestMeter(t *testing.T, r *Runtime) {
	t.Helper()
	if r.cfg.InstallationID == "" {
		r.cfg.InstallationID = newID()
	}
	if err := r.EnableJetStream(context.Background(), natsfixture.New(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
}
func projectTestUsage(t *testing.T, r *Runtime) {
	t.Helper()
	if err := r.ProjectUsage(context.Background()); err != nil {
		t.Fatal(err)
	}
}
