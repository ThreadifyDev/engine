package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/middleware"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"

	sharedauth "threadify-go/shared/auth"
)

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("APIKey_Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockAuthSvc := enginemocks.NewMockAuthService(ctrl)

		apiKey := "test-api-key"
		userInfo := &interfaces.UserInfo{
			OwnerID:   "user-123",
			CompanyID: "comp-123",
			Role:      "admin",
		}

		mockAuthSvc.EXPECT().
			ValidateApiKey(apiKey).
			Return(userInfo, nil)

		r := gin.New()
		r.Use(middleware.AuthMiddleware(mockAuthSvc, middleware.AuthAPIKey))
		r.GET("/test", func(c *gin.Context) {
			userID, _ := c.Get(sharedauth.CtxUserID)
			assert.Equal(t, userInfo.OwnerID, userID)
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("X-API-Key", apiKey)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("JWT_Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockAuthSvc := enginemocks.NewMockAuthService(ctrl)

		token := "valid-token"
		claims := &sharedauth.TokenClaims{
			UserID:    "user-123",
			CompanyID: "comp-123",
			Roles:     []string{"user"},
		}

		mockAuthSvc.EXPECT().
			VerifyToken(gomock.Any(), token).
			Return(claims, nil)
		mockAuthSvc.EXPECT().
			GetUserRoles(gomock.Any(), claims.UserID, "user", gomock.Any()).
			Return([]string{"admin"}, nil)

		r := gin.New()
		r.Use(middleware.AuthMiddleware(mockAuthSvc, middleware.AuthJWT))
		r.GET("/test", func(c *gin.Context) {
			userID, _ := c.Get(sharedauth.CtxUserID)
			assert.Equal(t, claims.UserID, userID)
			roles, _ := c.Get(sharedauth.CtxRoles)
			assert.Contains(t, roles, "admin")
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockAuthSvc := enginemocks.NewMockAuthService(ctrl)

		r := gin.New()
		r.Use(middleware.AuthMiddleware(mockAuthSvc, middleware.AuthDual))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "either X-API-Key or Authorization header required")
	})
}
