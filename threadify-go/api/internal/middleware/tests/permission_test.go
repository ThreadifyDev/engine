package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"threadify-go/api/internal/middleware"
	sa_mocks "threadify-go/api/internal/service/mocks/repository/serviceaccount"
	ur_mocks "threadify-go/api/internal/service/mocks/repository/userrole"
	pl_mocks "threadify-go/api/internal/service/mocks/service/permission"
	"threadify-go/shared/management/domain"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("User_Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockURRepo := ur_mocks.NewMockUserRoleRepository(ctrl)
		mockSARepo := sa_mocks.NewMockServiceAccountRepository(ctrl)
		mockPL := pl_mocks.NewMockPermissionLoader(ctrl)

		userID := "user-123"
		roleNames := []string{"admin"}
		permissions := []string{"contract.read"}

		mockURRepo.EXPECT().GetUserRoles(gomock.Any(), userID).Return(roleNames, nil)
		mockPL.EXPECT().GetPermissionsForRoles(roleNames, "app_level").Return(permissions)
		mockPL.EXPECT().CheckPermission(permissions, "contract.read").Return(true)

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("userID", userID)
			c.Next()
		})
		r.Use(middleware.RequirePermission(mockPL, mockSARepo, mockURRepo, "contract.read"))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("User_Forbidden", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockURRepo := ur_mocks.NewMockUserRoleRepository(ctrl)
		mockSARepo := sa_mocks.NewMockServiceAccountRepository(ctrl)
		mockPL := pl_mocks.NewMockPermissionLoader(ctrl)

		userID := "user-123"
		roleNames := []string{"standard"}
		permissions := []string{"other"}

		mockURRepo.EXPECT().GetUserRoles(gomock.Any(), userID).Return(roleNames, nil)
		mockPL.EXPECT().GetPermissionsForRoles(roleNames, "app_level").Return(permissions)
		mockPL.EXPECT().CheckPermission(permissions, "contract.read").Return(false)

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("userID", userID)
			c.Next()
		})
		r.Use(middleware.RequirePermission(mockPL, mockSARepo, mockURRepo, "contract.read"))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("ServiceAccount_Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockURRepo := ur_mocks.NewMockUserRoleRepository(ctrl)
		mockSARepo := sa_mocks.NewMockServiceAccountRepository(ctrl)
		mockPL := pl_mocks.NewMockPermissionLoader(ctrl)

		saID := "sa-123"
		roleNames := []string{"bot"}
		permissions := []string{"contract.read"}

		mockSARepo.EXPECT().FindByID(gomock.Any(), saID).Return(&domain.ServiceAccount{ID: saID, IsActive: true}, nil)
		mockURRepo.EXPECT().GetServiceAccountRoles(gomock.Any(), saID).Return(roleNames, nil)
		mockPL.EXPECT().GetPermissionsForRoles(roleNames, "runtime_level").Return(permissions)
		mockPL.EXPECT().CheckPermission(permissions, "contract.read").Return(true)

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("serviceAccountID", saID)
			c.Next()
		})
		r.Use(middleware.RequirePermission(mockPL, mockSARepo, mockURRepo, "contract.read"))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}
