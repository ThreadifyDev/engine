package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type EmailService struct {
	apiKey      string
	apiURL      string
	frontendURL string
	httpClient  *http.Client
}

func NewEmailService(apiKey, frontendURL string) *EmailService {
	return &EmailService{
		apiKey:      apiKey,
		apiURL:      "https://api.useplunk.com/v1/send",
		frontendURL: frontendURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type PlunkEmailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *EmailService) SendOTP(email, code string) error {
	emailBody := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2 style="color: #000;">Verify Your Email</h2>
			<p>Your verification code is:</p>
			<div style="background-color: #000; color: #fff; padding: 20px; text-align: center; font-size: 32px; font-weight: bold; letter-spacing: 8px; margin: 20px 0;">
				%s
			</div>
			<p>This code will expire in 10 minutes.</p>
			<p style="color: #666; font-size: 12px;">If you didn't request this code, please ignore this email.</p>
		</div>
	`, code)

	payload := PlunkEmailRequest{
		To:      email,
		Subject: "Verify Your Threadify Account",
		Body:    emailBody,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	req, err := http.NewRequest("POST", s.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("email service returned status %d", resp.StatusCode)
	}

	return nil
}

func (s *EmailService) SendWelcomeEmail(email, fullName string) error {
	dashboardLink := fmt.Sprintf("%s/u/dashboard", s.frontendURL)
	emailBody := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2 style="color: #000;">Welcome to Threadify!</h2>
			<p>Hi %s,</p>
			<p>Your account has been successfully verified. You can now start using Threadify to monitor and validate your business workflows.</p>
			<div style="margin: 30px 0;">
				<a href="%s" style="background-color: #000; color: #fff; padding: 12px 24px; text-decoration: none; display: inline-block;">
					Go to Dashboard
				</a>
			</div>
			<p>If you have any questions, feel free to reach out to our support team.</p>
			<p style="color: #666; font-size: 12px;">© 2026 Threadify. All rights reserved.</p>
		</div>
	`, fullName, dashboardLink)

	payload := PlunkEmailRequest{
		To:      email,
		Subject: "Welcome to Threadify",
		Body:    emailBody,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	req, err := http.NewRequest("POST", s.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("email service returned status %d", resp.StatusCode)
	}

	return nil
}

func (s *EmailService) SendPasswordResetEmail(email, resetToken string) error {
	// Construct reset link using configured frontend URL
	resetLink := fmt.Sprintf("%s/auth/reset-password?token=%s", s.frontendURL, resetToken)

	emailBody := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2 style="color: #000;">Reset Your Password</h2>
			<p>You requested to reset your password for your Threadify account.</p>
			<p>Click the button below to reset your password:</p>
			<div style="margin: 30px 0;">
				<a href="%s" style="background-color: #000; color: #fff; padding: 12px 24px; text-decoration: none; display: inline-block;">
					Reset Password
				</a>
			</div>
			<p>Or copy and paste this link into your browser:</p>
			<p style="word-break: break-all; color: #666; font-size: 12px;">%s</p>
			<p style="color: #666; margin-top: 30px;">This link will expire in 1 hour.</p>
			<p style="color: #666; font-size: 12px;">If you didn't request this password reset, please ignore this email. Your password will remain unchanged.</p>
		</div>
	`, resetLink, resetLink)

	payload := PlunkEmailRequest{
		To:      email,
		Subject: "Reset Your Threadify Password",
		Body:    emailBody,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	req, err := http.NewRequest("POST", s.apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("email service returned status %d", resp.StatusCode)
	}

	return nil
}
