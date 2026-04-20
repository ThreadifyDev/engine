package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	valkeyrepo "github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/service"
	"go.uber.org/zap"
)

func TestThreadServiceBuilder_Build_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(b *service.ThreadServiceBuilder)
		wantErr string
	}{
		{
			name:    "nil config",
			mutate:  func(b *service.ThreadServiceBuilder) { b.WithConfig(nil) },
			wantErr: "config is required",
		},
		{
			name:    "nil database",
			mutate:  func(b *service.ThreadServiceBuilder) { b.WithDatabase(nil) },
			wantErr: "database is required",
		},
		{
			name:    "nil valkey service",
			mutate:  func(b *service.ThreadServiceBuilder) { b.WithValkey(nil) },
			wantErr: "valkey service is required",
		},
		{
			name:    "nil thread repository",
			mutate:  func(b *service.ThreadServiceBuilder) { b.WithThreadRepository(nil) },
			wantErr: "thread repository is required",
		},
		{
			name:    "nil logger",
			mutate:  func(b *service.ThreadServiceBuilder) { b.WithLogger(nil) },
			wantErr: "logger is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := service.NewThreadServiceBuilder().
				WithConfig(new(config.Config)).
				WithDatabase(new(database.PostgresDB)).
				WithValkey(new(database.ValkeyService)).
				WithThreadRepository(new(valkeyrepo.ThreadRepository)).
				WithLogger(zap.NewNop())

			tc.mutate(b)
			_, err := b.Build()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestThreadServiceBuilder_WithMethods_ReturnSamePointer(t *testing.T) {
	b := service.NewThreadServiceBuilder()

	cfg := new(config.Config)
	assert.Same(t, b, b.WithConfig(cfg))

	logger := zap.NewNop()
	assert.Same(t, b, b.WithLogger(logger))

	assert.Same(t, b, b.WithContractTTL(3600))
}
