package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/service"
)

func TestInvitationTokenService_JWTWorkflow(t *testing.T) {
	const secret = "test-secret"
	const issuer = "test-issuer"
	svc := service.NewInvitationTokenService(secret, issuer)

	tests := []struct {
		name       string
		setup      func() string
		wantErr    bool
		validateFn func(t *testing.T, token string)
	}{
		{
			name: "valid token roundtrip",
			setup: func() string {
				token, err := svc.CreateToken("thread-1", "user-1", "participant", "observer", time.Hour)
				require.NoError(t, err)
				return token
			},
			validateFn: func(t *testing.T, token string) {
				claims, err := svc.ValidateToken(token)
				require.NoError(t, err)
				assert.Equal(t, "thread-1", claims.ThreadID)
				assert.Equal(t, "participant", claims.Role)
				assert.Equal(t, "observer", claims.AccessLevel)
				assert.Equal(t, "user-1", claims.InvitedBy)
				assert.Equal(t, issuer, claims.Issuer)
			},
		},
		{
			name: "invalid token string",
			setup: func() string {
				return "not.a.valid.token"
			},
			wantErr: true,
		},
		{
			name: "wrong signing secret",
			setup: func() string {
				token, _ := svc.CreateToken("t1", "u1", "r1", "observer", time.Hour)
				return token
			},
			wantErr: true,
			validateFn: func(t *testing.T, token string) {
				badSvc := service.NewInvitationTokenService("different-secret", issuer)
				_, err := badSvc.ValidateToken(token)
				assert.Error(t, err)
			},
		},
		{
			name: "expired token",
			setup: func() string {
				token, _ := svc.CreateToken("t1", "u1", "r1", "observer", -time.Minute)
				return token
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token := tc.setup()
			if tc.validateFn != nil {
				tc.validateFn(t, token)
				return
			}
			_, err := svc.ValidateToken(token)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInvitationTokenService_ParseExpiry(t *testing.T) {
	svc := service.NewInvitationTokenService("s", "i")

	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{input: "", expected: 24 * time.Hour},
		{input: "30m", expected: 30 * time.Minute},
		{input: "2h", expected: 2 * time.Hour},
		{input: "7d", expected: 7 * 24 * time.Hour},
		{input: "-1h", expected: -time.Hour},
		{input: "x", wantErr: true},
		{input: "30", wantErr: true},
		{input: "30z", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			d, err := svc.ParseExpiry(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, d)
		})
	}
}

func TestInvitationTokenService_ValidateAccessLevel(t *testing.T) {
	svc := service.NewInvitationTokenService("s", "i")

	tests := []struct {
		input   string
		wantErr bool
	}{
		{input: ""},
		{input: "owner"},
		{input: "participant"},
		{input: "observer"},
		{input: "external"},
		{input: "admin", wantErr: true},
		{input: "superuser", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			err := svc.ValidateAccessLevel(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInvitationTokenService_ValidateRole(t *testing.T) {
	svc := service.NewInvitationTokenService("s", "i")

	config := &service.InvitationConfig{
		AllowedRoles: []string{"developer", "manager"},
	}

	tests := []struct {
		name    string
		role    string
		config  *service.InvitationConfig
		wantErr bool
	}{
		{name: "allowed role developer", role: "developer", config: config},
		{name: "allowed role manager", role: "manager", config: config},
		{name: "disallowed role", role: "ceo", config: config, wantErr: true},
		{name: "nil config skips check", role: "anything", config: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.ValidateRole(tc.role, tc.config)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
