package models

// ActorInfo represents a resolved actor (user or service account)
type ActorInfo struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"` // "user" or "service_account"
	CompanyName *string `json:"companyName,omitempty"`
}
