package domain

// ActorInfo represents a resolved actor (user or service account)
type ActorInfo struct {
	ID          string
	Name        string
	Type        string // "user" or "service_account"
	CompanyName *string
}
