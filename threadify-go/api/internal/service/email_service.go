package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type EmailService struct {
	apiKey     string
	apiURL     string
	httpClient *http.Client
}

func NewEmailService(apiKey string) *EmailService {
	return &EmailService{
		apiKey: apiKey,
		apiURL: "https://api.useplunk.com/v1/send",
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
	emailBody := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2 style="color: #000;">Welcome to Threadify!</h2>
			<p>Hi %s,</p>
			<p>Your account has been successfully verified. You can now start using Threadify to monitor and validate your business workflows.</p>
			<div style="margin: 30px 0;">
				<a href="http://localhost:3000/dashboard" style="background-color: #000; color: #fff; padding: 12px 24px; text-decoration: none; display: inline-block;">
					Go to Dashboard
				</a>
			</div>
			<p>If you have any questions, feel free to reach out to our support team.</p>
			<p style="color: #666; font-size: 12px;">© 2026 Threadify. All rights reserved.</p>
		</div>
	`, fullName)

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
