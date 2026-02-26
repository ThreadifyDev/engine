package handlers

import (
	"errors"
	"net/http"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/service"
	"threadify-go/api/internal/validation"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
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

	if err := h.authService.VerifyEmail(c.Request.Context(), &req); err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, err.Error())
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Email has been successfully verified. You can now log in.",
	})
}

func authErrorResponse(err error, fallbackStatus int, fallbackMessage string) (int, string) {
	var requestValidationErr *validation.RequestValidationError
	if errors.As(err, &requestValidationErr) {
		return http.StatusBadRequest, requestValidationErr.FirstMessage()
	}

	switch {
	case errors.Is(err, service.ErrUserAlreadyExists):
		return http.StatusConflict, service.ErrUserAlreadyExists.Error()
	case errors.Is(err, service.ErrInvalidCredentials):
		return http.StatusUnauthorized, service.ErrInvalidCredentials.Error()
	case errors.Is(err, service.ErrInvalidEmail):
		return http.StatusBadRequest, service.ErrInvalidEmail.Error()
	case errors.Is(err, service.ErrExpiredToken):
		return http.StatusBadRequest, service.ErrExpiredToken.Error()
	case errors.Is(err, service.ErrInvalidToken):
		return http.StatusBadRequest, service.ErrInvalidToken.Error()
	case errors.Is(err, service.ErrRateLimit):
		return http.StatusTooManyRequests, service.ErrRateLimit.Error()
	default:
		return fallbackStatus, service.ErrInternalServerError.Error()
	}
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
