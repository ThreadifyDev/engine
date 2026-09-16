package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	sharedauth "threadify-go/shared/auth"
)

func TestCompanySettingsRequireAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, roles := range [][]string{nil, {"member"}, {"reader"}} {
		r := gin.New()
		r.POST("/profile", func(c *gin.Context) {
			c.Set(sharedauth.CtxUserID, "person")
			c.Set(sharedauth.CtxCompanyID, "company")
			c.Set(sharedauth.CtxRoles, roles)
		}, NewUserHandler(nil).UpdateProfile)
		req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(`{"full_name":"Test Person","job_role":"Engineer","industry":"SaaS"}`))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		if out.Code != http.StatusForbidden {
			t.Fatalf("roles %v: got %d: %s", roles, out.Code, out.Body)
		}
	}
}
