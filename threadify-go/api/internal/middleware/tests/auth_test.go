package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"threadify-go/api/internal/middleware"
	apikeymocks "threadify-go/api/internal/service/mocks/service/api_key"
	svcmocks "threadify-go/api/internal/service/mocks/service/auth"
	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestAuthAccessTokenAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAuthSvc := svcmocks.NewMockAuthService(ctrl)
		mockKeys := apikeymocks.NewMockAPIKeyService(ctrl)
		token := "valid-token"
		claims := &sharedauth.TokenClaims{
			UserID: "user-123",
			Roles:  []string{"user"},
		}

		mockAuthSvc.EXPECT().
			VerifyToken(gomock.Any(), token).
			Return(claims, nil)
		mockAuthSvc.EXPECT().
			GetUserRoles(gomock.Any(), claims.UserID, "user").
			Return([]string{"admin"}, nil)

		r := gin.New()
		r.Use(middleware.AuthAccessTokenAuth(mockAuthSvc, mockKeys))
		r.GET("/test", func(c *gin.Context) {
			userID := c.GetString(string(sharedauth.CtxUserID))
			assert.Equal(t, claims.UserID, userID)
			roles, _ := c.Get(string(sharedauth.CtxRoles))
			assert.Contains(t, roles.([]string), "admin")
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("MissingHeader", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAuthSvc := svcmocks.NewMockAuthService(ctrl)
		mockKeys := apikeymocks.NewMockAPIKeyService(ctrl)

		r := gin.New()
		r.Use(middleware.AuthAccessTokenAuth(mockAuthSvc, mockKeys))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Authorization header required")
	})

	t.Run("InvalidToken", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAuthSvc := svcmocks.NewMockAuthService(ctrl)
		mockKeys := apikeymocks.NewMockAPIKeyService(ctrl)
		token := "invalid-token"
		mockKeys.EXPECT().ValidateAPIKey(gomock.Any(), token).Return(nil, context.DeadlineExceeded)

		mockAuthSvc.EXPECT().
			VerifyToken(gomock.Any(), token).
			Return(nil, context.DeadlineExceeded)

		r := gin.New()
		r.Use(middleware.AuthAccessTokenAuth(mockAuthSvc, mockKeys))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
