package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxJSONBodyBytes = 1 << 20 // 1MB

func bindStrictJSON(c *gin.Context, dst any) error {
	if c.Request == nil || c.Request.Body == nil {
		return errors.New("request body is required")
	}

	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxJSONBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}

func respondBindError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	message := "Invalid request body"
	switch {
	case errors.Is(err, io.EOF):
		message = "Request body is required"
	case strings.Contains(err.Error(), "unknown field"):
		message = "Request body contains unknown fields"
	case strings.Contains(err.Error(), "cannot unmarshal"):
		message = "Request body contains invalid field types"
	case strings.Contains(err.Error(), "single JSON object"):
		message = "Request body must contain a single JSON object"
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": message})
}
