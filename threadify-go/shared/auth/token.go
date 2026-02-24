package auth

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	maxAuthorizationHeaderLength = 8192
	minBearerTokenLength         = 1
	maxBearerTokenLength         = 8192
)

var (
	ErrMissingAuthHeader = errors.New("authorization header required")

	ErrInvalidAuthFormat = errors.New("invalid authorization header format. Expected: Bearer <token>")

	ErrInvalidToken = errors.New("invalid token format")
)

func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ErrMissingAuthHeader
	}
	if len(authHeader) > maxAuthorizationHeaderLength {
		return "", ErrInvalidAuthFormat
	}

	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", ErrInvalidAuthFormat
	}

	token := parts[1]
	if len(token) < minBearerTokenLength || len(token) > maxBearerTokenLength {
		return "", ErrInvalidToken
	}
	if !utf8.ValidString(token) {
		return "", ErrInvalidToken
	}
	for _, r := range token {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", ErrInvalidToken
		}
	}

	return token, nil
}

func SetGinContextFromClaims(c *gin.Context, claims *TokenClaims) {
	c.Set(CtxUserID, claims.UserID)
	c.Set(CtxCompanyID, claims.CompanyID)
	c.Set(CtxEmail, claims.Email)
	c.Set(CtxRoles, claims.Roles)
	c.Set(CtxAuthSub, claims.Sub)
}
