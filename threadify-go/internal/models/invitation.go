package models

// InvitePartyRequest represents a request to create an invitation token
type InvitePartyRequest struct {
	Action      string `json:"action"`      // "inviteParty"
	Role        string `json:"role"`        // Required contract role
	Permissions string `json:"permissions"` // Optional, default "read,write"
	ExpiresIn   string `json:"expiresIn"`   // Optional, default "24h" (format: "24h", "2d", "30m")
}

// InvitePartyResponse represents the response from inviteParty
type InvitePartyResponse struct {
	Action      string `json:"action"`      // "inviteParty"
	Status      string `json:"status"`      // "success"|"error"
	ThreadToken string `json:"threadToken"` // JWT invitation token
	Role        string `json:"role"`        // Assigned role
	Permissions string `json:"permissions"` // Granted permissions
	ExpiresAt   int64  `json:"expiresAt"`   // Unix timestamp
	Message     string `json:"message"`     // Response message
}

// JoinThreadRequest represents a request to join a thread with token or directly
type JoinThreadRequest struct {
	Action      string `json:"action"`                // "joinThread"
	ThreadToken string `json:"threadToken,omitempty"` // JWT invitation token (for token-based join)
	ThreadID    string `json:"threadId,omitempty"`    // Thread ID (for direct join)
	Role        string `json:"role,omitempty"`        // Role (for direct join)
}

// JoinThreadResponse represents the response from joinThread
type JoinThreadResponse struct {
	Action      string `json:"action"`      // "joinThread"
	Status      string `json:"status"`      // "success"|"error"
	ThreadID    string `json:"threadId"`    // Joined thread ID
	ContractID  string `json:"contractId"`  // Parent contract ID
	Role        string `json:"role"`        // User role in thread
	Permissions string `json:"permissions"` // User permissions
	Message     string `json:"message"`     // Response message
}
