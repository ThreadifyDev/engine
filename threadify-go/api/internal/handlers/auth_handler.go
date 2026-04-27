package handlers

import (
	"errors"
	"net/http"
	"strings"
	iface "threadify-go/api/internal/interfaces"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/validation"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService iface.AuthService
}

func NewAuthHandler(authService iface.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) Signup(c *gin.Context) {
	var req models.SignupRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateSignupRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	if err := h.authService.Signup(c.Request.Context(), &req); err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, err.Error())
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Signup successful.",
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateLoginRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	authResp, err := h.authService.Login(c.Request.Context(), &req, c.ClientIP())
	if err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusUnauthorized, err.Error())
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, authResp)
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req models.ForgotPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateForgotPasswordRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	if err := h.authService.ForgotPassword(c.Request.Context(), &req); err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, _ := authErrorResponse(err, http.StatusInternalServerError, "Unable to process password reset request.")
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "If an account exists with this email, password reset instructions will be sent.",
	})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req models.ResetPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateResetPasswordRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	if err := h.authService.ResetPassword(c.Request.Context(), &req); err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, err.Error())
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Password has been successfully reset. You can now log in with your new password.",
	})
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req models.VerifyEmailRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateVerifyEmailRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	authResp, err := h.authService.VerifyEmail(c.Request.Context(), &req)
	if err != nil {
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, err.Error())
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, authResp)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.Status(http.StatusNoContent)
		return
	}

	token := parts[1]
	if err := h.authService.Logout(c.Request.Context(), token); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or revoked token"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) ResendVerificationEmail(c *gin.Context) {
	var req models.ResendVerificationEmailRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateResendVerificationEmailRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	if err := h.authService.ResendVerificationEmail(c.Request.Context(), &req); err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, err.Error())
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "If an account exists with this email, a verification email will be sent.",
	})
}

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
