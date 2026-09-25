package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/middleware"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	sharedauth "threadify-go/shared/auth"
)

func TestOAuthAccessCannotReachNonReadRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, scenario := range []struct{ method, path string }{
		{http.MethodPost, "/v1/contracts"},
		{http.MethodGet, "/v1/users"},
		{http.MethodGet, "/v1/contracts/c1/versions"},
		{http.MethodPost, "/graphql"},
		{http.MethodPost, "/sse"},
		{http.MethodGet, "/v1/agent/status"},
	} {
		t.Run(scenario.method+scenario.path, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mock := enginemocks.NewMockAuthService(ctrl)
			mock.EXPECT().VerifyToken(gomock.Any(), "toat_test").Return(&sharedauth.TokenClaims{UserID: "user", CompanyID: "company", Roles: []string{"admin"}, OAuthAccess: true, OAuthScopes: []string{"contract.read.*"}}, nil)
			mock.EXPECT().GetUserRoles(gomock.Any(), "user", "user", gomock.Any()).Return([]string{"admin"}, nil).AnyTimes()
			r := gin.New()
			r.Use(middleware.AuthMiddleware(mock, middleware.AuthJWT))
			r.Handle(scenario.method, scenario.path, func(c *gin.Context) { c.Status(http.StatusOK) })
			w := httptest.NewRecorder()
			req := httptest.NewRequest(scenario.method, scenario.path, nil)
			req.Header.Set("Authorization", "Bearer toat_test")
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, w.Body.String(), "OAuth scope does not permit this route")
		})
	}
}
