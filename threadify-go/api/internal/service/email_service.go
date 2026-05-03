package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strings"
	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/dto"
	sharedconfig "threadify-go/shared/config"
	"time"

	"go.uber.org/zap"
)

const (
	plunkProvider = "plunk"
)

//go:generate mockgen -package=svcmocks -destination=./mocks/service/email_service_mock.go -source=email.go
type EmailService interface {
	SendWelcomeEmail(ctx context.Context, email, fullName string) error
	SendVerificationEmail(ctx context.Context, email, token string) error
	SendLoginOTPEmail(ctx context.Context, email, token string) error
	SendPasswordResetEmail(ctx context.Context, email, resetToken string) error
	SendTeamInvitationEmail(ctx context.Context, email, role, inviteLink string) error
}

type EmailProvider interface {
	Send(ctx context.Context, req dto.EmailRequest) error
}

type emailService struct {
	provider    EmailProvider
	frontendURL string
	fromEmail   string
	templates   *template.Template
}

func NewEmailService(provider EmailProvider, frontendURL, fromEmail string) (EmailService, error) {
	tmpl, err := template.ParseFS(emailTemplates, "templates/email/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse email templates: %w", err)
	}
	return &emailService{
		provider:    provider,
		frontendURL: frontendURL,
		fromEmail:   fromEmail,
		templates:   tmpl,
	}, nil
}

func NewEmailServiceFromConfig(cfg *sharedconfig.Config, client *http.Client, logger *zap.Logger) (EmailService, error) {
	switch strings.ToLower(cfg.WebAPI.Email.Provider) {
	case plunkProvider:
		p := NewPlunkEmailProvider(cfg.WebAPI.Email.APIKey, cfg.WebAPI.Email.APIURL, client, logger)
		return NewEmailService(p, cfg.WebAPI.FrontendURL, cfg.WebAPI.Email.FromEmail)
	default:
		return nil, fmt.Errorf("unsupported email provider %q", cfg.WebAPI.Email.Provider)
	}
}

type plunkEmailProvider struct {
	apiKey     string
	apiURL     string
	httpClient *http.Client
	logger     *zap.Logger
}

func NewPlunkEmailProvider(apiKey, apiURL string, client *http.Client, logger *zap.Logger) EmailProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &plunkEmailProvider{
		apiKey:     apiKey,
		apiURL:     apiURL,
		httpClient: client,
		logger:     logger,
	}
}

func (p *plunkEmailProvider) Send(ctx context.Context, payload dto.EmailRequest) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal email payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Error("email: failed to send request", zap.Error(err))
		return fmt.Errorf("send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		p.logger.Error("email: provider returned error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", strings.TrimSpace(string(body))),
			zap.String("to", payload.To),
		)
		return fmt.Errorf("email service error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (s *emailService) SendWelcomeEmail(ctx context.Context, email, fullName string) error {
	body, err := s.render("welcome.html", domain.WelcomeData{
		Name:        firstNonEmpty(fullName, "there"),
		ActionURL:   fmt.Sprintf("%s/u/dashboard", s.frontendURL),
		FrontendURL: s.frontendURL,
		Year:        time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.provider.Send(ctx, dto.EmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "Welcome to Threadify",
		Body:    body,
	})
}

func (s *emailService) SendVerificationEmail(ctx context.Context, email, token string) error {
	body, err := s.render("verify.html", domain.VerifyData{
		ActionURL: fmt.Sprintf("%s/auth/verify-email?email=%s&token=%s",
			s.frontendURL, url.QueryEscape(email), url.QueryEscape(token)),
		Token:       token,
		FrontendURL: s.frontendURL,
		Year:        time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.provider.Send(ctx, dto.EmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "Verify Your Threadify Account",
		Body:    body,
	})
}

func (s *emailService) SendLoginOTPEmail(ctx context.Context, email, token string) error {
	body, err := s.render("login_otp.html", domain.LoginOTPData{
		Token:       token,
		FrontendURL: s.frontendURL,
		Year:        time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.provider.Send(ctx, dto.EmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "Your Threadify Login Code",
		Body:    body,
	})
}

func (s *emailService) SendPasswordResetEmail(ctx context.Context, email, resetToken string) error {
	body, err := s.render("reset_password.html", domain.ResetPasswordData{
		ActionURL: fmt.Sprintf("%s/auth/reset-password?token=%s",
			s.frontendURL, url.QueryEscape(resetToken)),
		Token:       resetToken,
		FrontendURL: s.frontendURL,
		Year:        time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.provider.Send(ctx, dto.EmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "Reset Your Threadify Password",
		Body:    body,
	})
}

func (s *emailService) SendTeamInvitationEmail(ctx context.Context, email, role, inviteLink string) error {
	body, err := s.render("team_invitation.html", domain.TeamInvitationData{
		Role:        role,
		ActionURL:   inviteLink,
		FrontendURL: s.frontendURL,
		Year:        time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.provider.Send(ctx, dto.EmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "You're invited to join Threadify",
		Body:    body,
	})
}

func (s *emailService) render(templateName string, data any) (string, error) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, templateName, data); err != nil {
		return "", fmt.Errorf("render template %s: %w", templateName, err)
	}
	return buf.String(), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
