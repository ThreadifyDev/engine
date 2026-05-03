package domain

type WelcomeData struct {
	Name        string
	ActionURL   string
	FrontendURL string
	Year        int
}

type VerifyData struct {
	ActionURL   string
	Token       string
	FrontendURL string
	Year        int
}

type LoginOTPData struct {
	Token       string
	FrontendURL string
	Year        int
}

type ResetPasswordData struct {
	ActionURL   string
	Token       string
	FrontendURL string
	Year        int
}

type TeamInvitationData struct {
	Role        string
	ActionURL   string
	FrontendURL string
	Year        int
}
