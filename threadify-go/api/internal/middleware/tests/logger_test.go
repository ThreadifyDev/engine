package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"threadify-go/api/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Success", func(t *testing.T) {
		logger := zaptest.NewLogger(t)
		r := gin.New()
		r.Use(middleware.RequestLogger(logger))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestRecoveryWithLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Recovery", func(t *testing.T) {
		logger := zaptest.NewLogger(t)

		r := gin.New()
		r.Use(middleware.RecoveryWithLogger(logger))
		r.GET("/panic", func(c *gin.Context) {
			panic("something went wrong")
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/panic", nil)

		assert.NotPanics(t, func() {
			r.ServeHTTP(w, req)
		})

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "an unexpected error occurred")
	})
}
