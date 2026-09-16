package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/management/domain"
	"threadify-go/shared/management/ports"
)

type streamFailureAgent struct {
	ports.AgentService
	emit bool
}

func (s streamFailureAgent) ChatStreamEino(ctx context.Context, auth, user, company, conversation, message, skill string, onEvent domain.StreamHandler) error {
	if s.emit {
		onEvent(domain.EventError, "AI assistant is unavailable because no model provider is configured.")
	}
	return errors.New("internal error should not be exposed")
}
func TestChat_ErrorEventSentOnce(t *testing.T) {
	for _, emit := range []bool{true, false} {
		t.Run(map[bool]string{true: "preserve_service_message", false: "fallback_without_service_event"}[emit], func(t *testing.T) {
			router := gin.New()
			router.POST("/chat", func(c *gin.Context) {
				c.Set(sharedauth.CtxUserID, "user")
				c.Set(sharedauth.CtxCompanyID, "company")
				NewAgentHandler(streamFailureAgent{emit: emit}).Chat(c)
			})
			req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"hello","skill":"auto"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, 1, strings.Count(rec.Body.String(), "event:error"))
			require.NotContains(t, rec.Body.String(), "internal error should not be exposed")
			if emit {
				require.Contains(t, rec.Body.String(), "no model provider is configured")
				require.NotContains(t, rec.Body.String(), "unexpected")
			}
		})
	}
}
