package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/middleware"
	apikeymocks "threadify-go/api/internal/service/mocks/service/api_key"
	authmocks "threadify-go/api/internal/service/mocks/service/auth"
	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func authRouter(auth *authmocks.MockAuthService, keys *apikeymocks.MockAPIKeyService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthAccessTokenAuth(auth, keys))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":            c.GetString(sharedauth.CtxUserID),
			"company_id":         c.GetString(sharedauth.CtxCompanyID),
			"service_account_id": c.GetString("serviceAccountID"),
		})
	})
	return router
}

func performAuthRequest(router *gin.Engine, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAuthAccessTokenAuth_PreservesJWTAuthentication(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth, keys := authmocks.NewMockAuthService(ctrl), apikeymocks.NewMockAPIKeyService(ctrl)
	auth.EXPECT().VerifyToken(gomock.Any(), "jwt").Return(&sharedauth.TokenClaims{UserID: "user", CompanyID: "company"}, nil)
	auth.EXPECT().GetUserRoles(gomock.Any(), "user", "user").Return([]string{"owner"}, nil)

	response := performAuthRequest(authRouter(auth, keys), "jwt")

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"user_id":"user"`)
}

func TestAuthAccessTokenAuth_FallsBackToServiceAPIKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth, keys := authmocks.NewMockAuthService(ctrl), apikeymocks.NewMockAPIKeyService(ctrl)
	serviceAccountID := "service-account"
	auth.EXPECT().VerifyToken(gomock.Any(), "api-key").Return(nil, errors.New("not a jwt"))
	keys.EXPECT().ValidateAPIKey(gomock.Any(), "api-key").Return(&domain.APIKey{
		CompanyID: "company", ServiceAccountID: &serviceAccountID,
	}, nil)

	response := performAuthRequest(authRouter(auth, keys), "api-key")

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"service_account_id":"service-account"`)
	assert.Contains(t, response.Body.String(), `"company_id":"company"`)
}

func TestAuthAccessTokenAuth_RejectsInvalidJWTAndAPIKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth, keys := authmocks.NewMockAuthService(ctrl), apikeymocks.NewMockAPIKeyService(ctrl)
	auth.EXPECT().VerifyToken(gomock.Any(), "invalid").Return(nil, errors.New("invalid jwt"))
	keys.EXPECT().ValidateAPIKey(gomock.Any(), "invalid").Return(nil, errors.New("invalid key"))

	response := performAuthRequest(authRouter(auth, keys), "invalid")

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}
