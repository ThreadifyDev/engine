package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/middleware"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
)

func TestIPRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Allowed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockLua := enginemocks.NewMockLuaScriptManager(ctrl)

		rateCfg := &config.RateLimitConfig{
			Enabled:             true,
			IPRateLimitEnabled:  true,
			IPRequestsPerWindow: 10,
			WindowSeconds:       60,
		}

		ip := "127.0.0.1"
		mockLua.EXPECT().
			CheckIPRateLimit(gomock.Any(), ip, rateCfg.IPRequestsPerWindow, rateCfg.WindowSeconds).
			Return(true, nil)

		r := gin.New()
		r.Use(middleware.IPRateLimitMiddleware(mockLua, rateCfg))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = ip + ":1234"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Blocked", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockLua := enginemocks.NewMockLuaScriptManager(ctrl)

		rateCfg := &config.RateLimitConfig{
			Enabled:             true,
			IPRateLimitEnabled:  true,
			IPRequestsPerWindow: 10,
			WindowSeconds:       60,
		}

		ip := "127.0.0.1"
		mockLua.EXPECT().
			CheckIPRateLimit(gomock.Any(), ip, rateCfg.IPRequestsPerWindow, rateCfg.WindowSeconds).
			Return(false, nil)

		r := gin.New()
		r.Use(middleware.IPRateLimitMiddleware(mockLua, rateCfg))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = ip + ":1234"
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Contains(t, w.Body.String(), "Too many requests")
	})
}
