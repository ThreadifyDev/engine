package valkey

import (
	"time"

	"github.com/threadify/engine/internal/domain"
)

// connectedClientModel is the repository-specific model for ConnectedClient with JSON tags for Valkey storage
type connectedClientModel struct {
	OwnerID          string    `json:"ownerId"`
	CompanyID        string    `json:"companyId"`
	ApiKey           string    `json:"apiKey"`
	ServiceName      string    `json:"serviceName"`
	ConnectedAt      time.Time `json:"connectedAt"`
	SubscribedEvents []string  `json:"subscribedEvents"`
}

func fromConnectedClientDomain(d *domain.ConnectedClient) *connectedClientModel {
	if d == nil {
		return nil
	}
	return &connectedClientModel{
		OwnerID:          d.OwnerID,
		CompanyID:        d.CompanyID,
		ApiKey:           d.ApiKey,
		ServiceName:      d.ServiceName,
		ConnectedAt:      d.ConnectedAt,
		SubscribedEvents: d.SubscribedEvents,
	}
}

func (m *connectedClientModel) ToDomain() *domain.ConnectedClient {
	if m == nil {
		return nil
	}
	return &domain.ConnectedClient{
		OwnerID:          m.OwnerID,
		CompanyID:        m.CompanyID,
		ApiKey:           m.ApiKey,
		ServiceName:      m.ServiceName,
		ConnectedAt:      m.ConnectedAt,
		SubscribedEvents: m.SubscribedEvents,
	}
}

// stepStateSnapshotModel is the repository-specific model for StepStateSnapshot with JSON tags for Valkey storage
type stepStateSnapshotModel struct {
	ID             string     `json:"id"`
	ThreadID       string     `json:"thread_id"`
	StepName       string     `json:"step_name"`
	IdempotencyKey string     `json:"idempotency_key"`
	Status         string     `json:"status"`
	RetryCount     int        `json:"retry_count"`
	FirstSeenAt    time.Time  `json:"first_seen_at"`
	LastUpdatedAt  time.Time  `json:"last_updated_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	PreviousStep   string     `json:"previous_step,omitempty"`
	Actor          string     `json:"actor,omitempty"`
	ActorService   string     `json:"actor_service,omitempty"`
	LatestContext  string     `json:"latest_context,omitempty"`
}

func fromStepStateSnapshotDomain(d domain.StepStateSnapshot) stepStateSnapshotModel {
	return stepStateSnapshotModel{
		ID:             d.ID,
		ThreadID:       d.ThreadID,
		StepName:       d.StepName,
		IdempotencyKey: d.IdempotencyKey,
		Status:         d.Status,
		RetryCount:     d.RetryCount,
		FirstSeenAt:    d.FirstSeenAt,
		LastUpdatedAt:  d.LastUpdatedAt,
		StartedAt:      d.StartedAt,
		FinishedAt:     d.FinishedAt,
		PreviousStep:   d.PreviousStep,
		Actor:          d.Actor,
		ActorService:   d.ActorService,
		LatestContext:  d.LatestContext,
	}
}

func (m *stepStateSnapshotModel) ToDomain() domain.StepStateSnapshot {
	return domain.StepStateSnapshot{
		ID:             m.ID,
		ThreadID:       m.ThreadID,
		StepName:       m.StepName,
		IdempotencyKey: m.IdempotencyKey,
		Status:         m.Status,
		RetryCount:     m.RetryCount,
		FirstSeenAt:    m.FirstSeenAt,
		LastUpdatedAt:  m.LastUpdatedAt,
		StartedAt:      m.StartedAt,
		FinishedAt:     m.FinishedAt,
		PreviousStep:   m.PreviousStep,
		Actor:          m.Actor,
		ActorService:   m.ActorService,
		LatestContext:  m.LatestContext,
	}
}

// userAccessModel is the repository-specific model for UserAccess with JSON tags for Valkey storage
type userAccessModel struct {
	Roles       []string   `json:"roles"`
	RuntimeRole string     `json:"runtime_role"`
	Permissions []string   `json:"permissions"`
	GrantedBy   string     `json:"granted_by"`
	GrantedAt   time.Time  `json:"granted_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	Status      string     `json:"status"`
}

func fromUserAccessDomain(d *domain.UserAccess) *userAccessModel {
	if d == nil {
		return nil
	}
	return &userAccessModel{
		Roles:       d.Roles,
		RuntimeRole: d.RuntimeRole,
		Permissions: d.Permissions,
		GrantedBy:   d.GrantedBy,
		GrantedAt:   d.GrantedAt,
		UpdatedAt:   d.UpdatedAt,
		Status:      d.Status,
	}
}

func (m *userAccessModel) ToDomain() *domain.UserAccess {
	if m == nil {
		return nil
	}
	return &domain.UserAccess{
		Roles:       m.Roles,
		RuntimeRole: m.RuntimeRole,
		Permissions: m.Permissions,
		GrantedBy:   m.GrantedBy,
		GrantedAt:   m.GrantedAt,
		UpdatedAt:   m.UpdatedAt,
		Status:      m.Status,
	}
}

// threadModel is the repository-specific model for Thread with JSON tags for Valkey storage
type threadModel struct {
	ID              string            `json:"id"`
	ContractID      *string           `json:"contractId,omitempty"`
	ContractVersion *int              `json:"contractVersion,omitempty"`
	ContractName    string            `json:"contractName,omitempty"`
	Refs            map[string]string `json:"refs,omitempty"`
	OwnerID         string            `json:"ownerId"`
	CompanyID       string            `json:"companyId"`
	Label           string            `json:"label,omitempty"`
	CreatedBy       string            `json:"createdBy,omitempty"`
	Status          string            `json:"status"`
	LastHash        string            `json:"lastHash,omitempty"`
	StartedAt       time.Time         `json:"startedAt"`
	CompletedAt     *time.Time        `json:"completedAt,omitempty"`
	Error           string            `json:"error,omitempty"`
}

func fromThreadDomain(t *domain.Thread) *threadModel {
	if t == nil {
		return nil
	}
	return &threadModel{
		ID:              t.ID,
		ContractID:      t.ContractID,
		ContractVersion: t.ContractVersion,
		ContractName:    t.ContractName,
		Refs:            t.Refs,
		OwnerID:         t.OwnerID,
		CompanyID:       t.CompanyID,
		Label:           t.Label,
		CreatedBy:       t.CreatedBy,
		Status:          string(t.Status),
		LastHash:        t.LastHash,
		StartedAt:       t.StartedAt,
		CompletedAt:     t.CompletedAt,
		Error:           t.Error,
	}
}

func (m *threadModel) ToDomain() *domain.Thread {
	if m == nil {
		return nil
	}
	return &domain.Thread{
		ID:              m.ID,
		ContractID:      m.ContractID,
		ContractVersion: m.ContractVersion,
		ContractName:    m.ContractName,
		Refs:            m.Refs,
		OwnerID:         m.OwnerID,
		CompanyID:       m.CompanyID,
		Label:           m.Label,
		CreatedBy:       m.CreatedBy,
		Status:          domain.ThreadStatus(m.Status),
		LastHash:        m.LastHash,
		StartedAt:       m.StartedAt,
		CompletedAt:     m.CompletedAt,
		Error:           m.Error,
	}
}


