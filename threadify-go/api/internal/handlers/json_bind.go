package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const maxRequestBodyBytes = 1 << 20 // 1MB

type apiError struct {
	Error string `json:"error"`
}

func bindJSON(c *gin.Context, dst any) bool {
	if err := decodeJSONRequest(c, dst); err != nil {
		c.JSON(http.StatusBadRequest, apiError{Error: err.Error()})
		return false
	}
	return true
}

func decodeJSONRequest(c *gin.Context, dst any) error {
	r := c.Request
	if r.Body == nil {
		return errors.New("request body is required")
	}

	r.Body = http.MaxBytesReader(c.Writer, r.Body, maxRequestBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return friendlyJSONError(err)
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}

func friendlyJSONError(err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var maxBytesErr *http.MaxBytesError

	switch {
	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		return errors.New("request body is required or malformed")

	case errors.As(err, &syntaxErr):
		return fmt.Errorf("malformed JSON at position %d", syntaxErr.Offset)

	case errors.As(err, &typeErr):
		return fmt.Errorf("field '%s' must be of type %s", typeErr.Field, typeErr.Type)

	case errors.As(err, &maxBytesErr):
		return fmt.Errorf("request body must not exceed %dMB", maxRequestBodyBytes/(1<<20))

	case isUnknownFieldError(err):
		return errors.New("request body contains unknown fields")

	default:
		return errors.New("invalid request body")
	}
}

func isUnknownFieldError(err error) bool {
	return err != nil && len(err.Error()) > 0 &&
		errors.Is(err, err) && // ensure it's a real error
		(len(err.Error()) >= 13 && err.Error()[:13] == "json: unknown")
}
