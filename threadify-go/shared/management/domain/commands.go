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
	MiddleName      string
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

// ApplyEntityProfileTypeCmd is the complete desired state of an entity profile
// type. Metric database IDs are deliberately absent from this contract: metrics
// are reconciled by their externally visible names.
type ApplyEntityProfileTypeCmd struct {
	Name        string
	Type        []string
	Description string
	Metrics     []EntityTypeMetric
}

type EntityProfileTypeChanges struct {
	NameChanged        bool
	DescriptionChanged bool
	TypesChanged       bool
	MetricsAdded       []string
	MetricsUpdated     []string
	MetricsRemoved     []string
}

type EntityProfileTypeBackfill struct {
	Supported bool
	Applied   bool
}

type ApplyEntityProfileTypeResult struct {
	Status     string
	DryRun     bool
	ConfigHash string
	Profile    *EntityProfileType
	Changes    EntityProfileTypeChanges
	Backfill   EntityProfileTypeBackfill
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
