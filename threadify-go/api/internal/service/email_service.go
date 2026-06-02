// service/email.go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"
)

//go:generate mockgen -package=svcmocks -destination=./mocks/service/email_service_mock.go -source=email_service.go
type EmailService interface {
	SendWelcomeEmail(ctx context.Context, email, fullName string) error
	SendVerificationEmail(ctx context.Context, email, token string) error
	SendLoginOTPEmail(ctx context.Context, email, token string) error
	SendPasswordResetEmail(ctx context.Context, email, resetToken string) error
	SendTeamInvitationEmail(ctx context.Context, email, role, inviteLink string) error
}

type plunkEmailService struct {
	apiKey      string
	apiURL      string
	frontendURL string
	fromEmail   string
	httpClient  *http.Client
	templates   *template.Template
}

func NewEmailService(apiKey, apiURL, frontendURL, fromEmail string) (EmailService, error) {
	tmpl, err := template.ParseFS(emailTemplates, "templates/email/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse email templates: %w", err)
	}

	return &plunkEmailService{
		apiKey:      apiKey,
		apiURL:      apiURL,
		frontendURL: frontendURL,
		fromEmail:   fromEmail,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		templates:   tmpl,
	}, nil
}

type emailData struct {
	Name        string
	ActionURL   string
	Token       string
	FrontendURL string
	Year        int
}

type plunkEmailRequest struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *plunkEmailService) SendWelcomeEmail(ctx context.Context, email, fullName string) error {
	name := firstNonEmpty(fullName, "there")
	body, err := s.render("welcome.html", emailData{
		Name:        name,
		ActionURL:   fmt.Sprintf("%s/u/dashboard", s.frontendURL),
		FrontendURL: s.frontendURL,
		Year:        time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.send(ctx, plunkEmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "Welcome to Threadify",
		Body:    body,
	})
}

func (s *plunkEmailService) SendVerificationEmail(ctx context.Context, email, token string) error {
	body, err := s.render("verify.html", emailData{
		ActionURL:   fmt.Sprintf("%s/auth/verify-email?email=%s", s.frontendURL, email),
		Token:       token,
		FrontendURL: s.frontendURL,
		Year:        time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.send(ctx, plunkEmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "Verify Your Threadify Account",
		Body:    body,
	})
}

func (s *plunkEmailService) SendLoginOTPEmail(ctx context.Context, email, token string) error {
	body, err := s.render("login_otp.html", emailData{
		Token: token,
		Year:  time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.send(ctx, plunkEmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "Your Threadify Login Code",
		Body:    body,
	})
}

func (s *plunkEmailService) SendPasswordResetEmail(ctx context.Context, email, resetToken string) error {
	body, err := s.render("reset_password.html", emailData{
		ActionURL:   fmt.Sprintf("%s/auth/reset-password", s.frontendURL),
		Token:       resetToken,
		FrontendURL: s.frontendURL,
		Year:        time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.send(ctx, plunkEmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "Reset Your Threadify Password",
		Body:    body,
	})
}

func (s *plunkEmailService) SendTeamInvitationEmail(ctx context.Context, email, role, inviteLink string) error {
	body, err := s.render("team_invitation.html", emailData{
		ActionURL:   inviteLink,
		FrontendURL: s.frontendURL,
		Year:        time.Now().Year(),
	})
	if err != nil {
		return err
	}
	return s.send(ctx, plunkEmailRequest{
		To:      email,
		From:    s.fromEmail,
		Subject: "You're invited to join Threadify",
		Body:    body,
	})
}

func (s *plunkEmailService) render(templateName string, data emailData) (string, error) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, templateName, data); err != nil {
		return "", fmt.Errorf("render template %s: %w", templateName, err)
	}
	return buf.String(), nil
}

func (s *plunkEmailService) send(ctx context.Context, payload plunkEmailRequest) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal email payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("email service error (status %d)", resp.StatusCode)
	}

	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
