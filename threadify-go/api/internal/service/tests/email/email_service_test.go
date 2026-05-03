package email

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"threadify-go/api/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEmailService_AllTemplates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	provider := service.NewPlunkEmailProvider("test-key", server.URL, nil, zap.NewNop())
	svc, err := service.NewEmailService(provider, "https://frontend.com", "noreply@threadify.dev")
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "SendWelcomeEmail",
			call: func() error { return svc.SendWelcomeEmail(ctx, "test@example.com", "John Doe") },
		},
		{
			name: "SendVerificationEmail",
			call: func() error { return svc.SendVerificationEmail(ctx, "test@example.com", "token-123") },
		},
		{
			name: "SendLoginOTPEmail",
			call: func() error { return svc.SendLoginOTPEmail(ctx, "test@example.com", "123456") },
		},
		{
			name: "SendPasswordResetEmail",
			call: func() error { return svc.SendPasswordResetEmail(ctx, "test@example.com", "reset-token") },
		},
		{
			name: "SendTeamInvitationEmail",
			call: func() error {
				return svc.SendTeamInvitationEmail(ctx, "test@example.com", "admin", "https://link.com")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			assert.NoError(t, err)
		})
	}
}

func TestEmailService_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`Server Error`))
	}))
	defer server.Close()

	provider := service.NewPlunkEmailProvider("test-key", server.URL, nil, zap.NewNop())
	svc, err := service.NewEmailService(provider, "https://frontend.com", "noreply@threadify.dev")
	require.NoError(t, err)

	err = svc.SendWelcomeEmail(context.Background(), "test@example.com", "John Doe")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}
