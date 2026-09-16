package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/management/validation"
)

func authErrorResponse(err error, fallbackStatus int, fallbackMessage string) (int, string) {
	if de := serror.GetDomainError(err); de != nil {
		return de.Code, de.Message
	}

	return fallbackStatus, fallbackMessage
}

func respondValidationError(c *gin.Context, err error) bool {
	var requestValidationErr *validation.RequestValidationError
	if !errors.As(err, &requestValidationErr) {
		return false
	}

	c.JSON(http.StatusBadRequest, gin.H{
		"error":   "Validation failed",
		"details": requestValidationErr.Problems(),
	})
	return true
}

func ctxString(c *gin.Context, key string) (string, bool) {
	raw, exists := c.Get(key)
	if !exists {
		return "", false
	}
	val, ok := raw.(string)
	return val, ok && val != ""
}
