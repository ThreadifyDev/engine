package handlers

import (
	"errors"
	"net/http"
	"strings"
	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/dto"
	"threadify-go/api/internal/metrics"
	"threadify-go/api/internal/ports"
	"threadify-go/api/internal/validation"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService ports.AuthService
}

func NewAuthHandler(authService ports.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) Signup(c *gin.Context) {
	var req dto.SignupRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateSignupRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	// Honeypot check
	if strings.TrimSpace(req.MiddleName) != "" {
		metrics.BotDetectionsTotal.WithLabelValues("middle_name").Inc()
		// Return 201 to make the bot think it succeeded
		c.JSON(http.StatusCreated, gin.H{
			"message": "Signup successful.",
		})
		return
	}

	if err := h.authService.Signup(c.Request.Context(), &domain.SignupCmd{
		CompanyName:     req.CompanyName,
		Email:           req.Email,
		Password:        req.Password,
		FullName:        req.FullName,
		JobRole:         req.JobRole,
		Industry:        req.Industry,
		CompanySize:     req.CompanySize,
		UseCase:         req.UseCase,
		InvitationToken: req.InvitationToken,
		MiddleName:      req.MiddleName,
	}); err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, "An unexpected error occurred")
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Signup successful.",
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateLoginRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	authResp, err := h.authService.Login(c.Request.Context(), &domain.LoginCmd{Email: req.Email, Password: req.Password}, c.ClientIP())
	if err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusUnauthorized, "Invalid credentials")
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, mapAuthResponseToDTO(authResp))
}

func mapAuthResponseToDTO(resp *domain.AuthSession) *dto.AuthResponse {
	if resp == nil {
		return nil
	}
	return &dto.AuthResponse{
		Email:                     resp.Email,
		Token:                     resp.Token,
		User:                      mapAuthUserToDTO(resp.User),
		OTPRequired:               resp.OTPRequired,
		EmailVerificationRequired: resp.EmailVerificationRequired,
		Message:                   resp.Message,
	}
}

func mapAuthUserToDTO(user *domain.User) *dto.AuthUser {
	if user == nil {
		return nil
	}
	return &dto.AuthUser{
		ID:                       user.ID,
		CompanyID:                user.CompanyID,
		Email:                    user.Email,
		FullName:                 user.FullName,
		JobRole:                  user.JobRole,
		EmailVerified:            user.EmailVerified,
		OnboardingCompleted:      user.OnboardingCompleted,
		FirstInstrumentationDone: user.FirstInstrumentationDone,
		CreatedAt:                user.CreatedAt,
		UpdatedAt:                user.UpdatedAt,
		LastLoginAt:              user.LastLoginAt,
	}
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateForgotPasswordRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	if err := h.authService.ForgotPassword(c.Request.Context(), &domain.ForgotPasswordCmd{Email: req.Email}); err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, "Unable to process password reset request.")
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "If an account exists with this email, password reset instructions will be sent.",
	})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateResetPasswordRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	if err := h.authService.ResetPassword(c.Request.Context(), &domain.ResetPasswordCmd{Token: req.Token, Password: req.Password}); err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, "An unexpected error occurred")
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Password has been successfully reset. You can now log in with your new password.",
	})
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req dto.VerifyEmailRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateVerifyEmailRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	authResp, err := h.authService.VerifyEmail(c.Request.Context(), &domain.VerifyEmailCmd{Email: req.Email, Token: req.Token})
	if err != nil {
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, "An unexpected error occurred")
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, mapAuthResponseToDTO(authResp))
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
	var req dto.ResendVerificationEmailRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := validation.ValidateResendVerificationEmailRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	if err := h.authService.ResendVerificationEmail(c.Request.Context(), &domain.ResendVerificationEmailCmd{Email: req.Email}); err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, "An unexpected error occurred")
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
