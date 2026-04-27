package engine

import (
	"testing"

	"github.com/threadify/engine/tests/internal/enginetest"
)

func setupTestUser(t *testing.T) *enginetest.TestUser {
	t.Helper()
	return enginetest.SetupTestUser(t, env, supabase)
}
