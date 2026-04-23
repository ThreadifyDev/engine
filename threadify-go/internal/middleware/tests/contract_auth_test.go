package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/middleware"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/rbac"
)

func TestContractRBACMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("PermissionAllowed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRBAC := enginemocks.NewMockRBACLoader(ctrl)

		roles := []string{"admin"}
		permission := "contract.read"

		mockRBAC.EXPECT().
			GetRolesByLevel("app_level").
			Return(map[string]rbac.Role{
				"admin": {
					Name:        "admin",
					Permissions: []string{"contract.read"},
				},
			})
		mockRBAC.EXPECT().
			CheckPermission([]string{"contract.read"}, permission).
			Return(true)

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(sharedauth.CtxUserID, "user-123")
			c.Set(sharedauth.CtxRoles, roles)
			c.Next()
		})
		r.Use(middleware.ContractRBACMiddleware(mockRBAC, permission))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("PermissionDenied", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockRBAC := enginemocks.NewMockRBACLoader(ctrl)

		roles := []string{"user"}
		permission := "contract.delete"

		mockRBAC.EXPECT().
			GetRolesByLevel("app_level").
			Return(map[string]rbac.Role{
				"user": {
					Name:        "user",
					Permissions: []string{"contract.read"},
				},
			})
		mockRBAC.EXPECT().
			CheckPermission([]string{"contract.read"}, "contract.delete").
			Return(false)
		mockRBAC.EXPECT().
			CheckPermission([]string{"contract.read"}, "contract.delete").
			Return(false) // It checks twice in the loop logic

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(sharedauth.CtxUserID, "user-123")
			c.Set(sharedauth.CtxRoles, roles)
			c.Next()
		})
		r.Use(middleware.ContractRBACMiddleware(mockRBAC, permission))
		r.DELETE("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("DELETE", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "Insufficient permissions")
	})
}
