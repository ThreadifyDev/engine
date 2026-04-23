package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/middleware"
)

func TestCORSMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("AllowAll", func(t *testing.T) {
		r := gin.New()
		r.Use(middleware.CORSMiddleware(""))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "http://any-origin.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		// With AllowAllOrigins: true, it returns "*"
		assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("WhitelistAllowed", func(t *testing.T) {
		r := gin.New()
		r.Use(middleware.CORSMiddleware("http://allowed.com, https://another.com"))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "http://allowed.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "http://allowed.com", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("WhitelistRejected", func(t *testing.T) {
		r := gin.New()
		r.Use(middleware.CORSMiddleware("http://allowed.com"))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("OPTIONS", "/test", nil)
		req.Header.Set("Origin", "http://malicious.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		r.ServeHTTP(w, req)

		// Rejected origins return 403 Forbidden
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})
}
