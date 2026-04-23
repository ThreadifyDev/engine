package tests

import (
	"embed"
	"net/http"
	"testing"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
	"threadify-go/shared/rbac"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/permissions.json testdata/roles.json
var roleRBACFS embed.FS

func newRoleRouter(t *testing.T) *gin.Engine {
	loader, err := rbac.NewLoaderFromFS(roleRBACFS, "testdata/permissions.json", "testdata/roles.json")
	require.NoError(t, err)

	h := handlers.NewRoleHandler(loader)
	r := common.SetupTestRouter()
	r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: "comp_1", UserID: "user_1"}))
	r.GET("/roles", h.GetRoles)
	r.GET("/roles/level/:level", h.GetRolesByLevel)
	return r
}

func TestRoleHandler(t *testing.T) {
	r := newRoleRouter(t)

	t.Run("GetRoles_Success", func(t *testing.T) {
		w := common.DoRequest(t, r, "GET", "/roles", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Roles map[string]interface{} `json:"roles"`
		}
		common.UnmarshalBody(t, w.Body.Bytes(), &resp)
		assert.NotNil(t, resp.Roles)
		assert.NotNil(t, resp.Roles["app_level"])
		assert.NotNil(t, resp.Roles["api_level"])
		assert.NotNil(t, resp.Roles["runtime_level"])
	})

	t.Run("GetRolesByLevel_Success", func(t *testing.T) {
		for _, level := range []string{"app_level", "api_level", "runtime_level"} {
			w := common.DoRequest(t, r, "GET", "/roles/level/"+level, nil)
			assert.Equal(t, http.StatusOK, w.Code)

			var resp struct {
				Level string               `json:"level"`
				Roles map[string]rbac.Role `json:"roles"`
			}
			common.UnmarshalBody(t, w.Body.Bytes(), &resp)
			assert.NotEmpty(t, resp.Roles)
			assert.Equal(t, level, resp.Level)
		}
	})

	t.Run("GetRolesByLevel_Invalid", func(t *testing.T) {
		w := common.DoRequest(t, r, "GET", "/roles/level/invalid", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
