package domain

type SignupCmd struct {
	CompanyName     string
	Email           string
	Password        string
	FullName        string
	JobRole         string
	Industry        *string
	CompanySize     *string
	UseCase         *string
	InvitationToken *string
}

type LoginCmd struct {
	Email    string
	Password string
}

type ForgotPasswordCmd struct {
	Email string
}

type ResetPasswordCmd struct {
	Token    string
	Password string
}

type VerifyEmailCmd struct {
	Email string
	Token string
}

type ResendVerificationEmailCmd struct {
	Email string
}

type AuthSession struct {
	Email                     string
	Token                     string
	User                      *User
	OTPRequired               bool
	EmailVerificationRequired bool
	Message                   string
}

type CreateAPIKeyCmd struct {
	Name                 string
	ExpiresIn            *int
	ServiceAccountID     *string
	CreateServiceAccount bool
	ServiceAccountRole   *string
}

type APIKeyCredentials struct {
	Key       string
	KeyPrefix string
	APIKey    *APIKey
}

type InvitationTokenInfo struct {
	CompanyName string
	Email       string
}

type CreateServiceAccountCmd struct {
	Name        string
	Description *string
	Role        string
}

type UpdateServiceAccountCmd struct {
	Name        string
	Description string
	IsActive    bool
}

type CreateEntityProfileTypeCmd struct {
	Name        string
	Type        []string
	Description string
	Metrics     []EntityTypeMetric
}

type UpdateEntityProfileTypeCmd struct {
	Name              string
	Type              []string
	Description       string
	Metrics           []EntityTypeMetric
	MarkedForDeletion []string
	ModifiedMetricIDs []string
}

type UpdateProfileCmd struct {
	FullName    *string
	JobRole     *string
	Industry    *string
	CompanySize *string
	UseCase     *string
}

type UserProfile struct {
	User    *User
	Company *Company
}
