package models

import "time"

type EntityProfileType struct {
	ID          string     `json:"id"`
	CompanyID   string     `json:"company_id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Description string     `json:"description,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type EntityProfile struct {
	ID            string    `json:"id"`
	RefKey        string    `json:"ref_key"`
	CompanyID     string    `json:"company_id"`
	ProfileTypeID string    `json:"profile_type_id"`
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"created_at"`
	LastActiveAt  time.Time `json:"last_active_at"`
}

type EntityProfileMetrics struct {
	EntityProfileID         string     `json:"entity_profile_id"`
	TotalDeliveries         int        `json:"total_deliveries"`
	CompletedSuccessfully   int        `json:"completed_successfully"`
	ValidationViolations    int        `json:"validation_violations"`
	DeliveryHealthScore     *float64   `json:"delivery_health_score"`
	PrevDeliveryHealthScore *float64   `json:"prev_delivery_health_score,omitempty"`
	HealthTrendSlope        *float64   `json:"health_trend_slope"`
	AverageDeliveryTimeMs   *int64     `json:"average_delivery_time_ms"`
	LastCalculatedAt        *time.Time `json:"last_calculated_at"`
}

type EntityPartnerCompatibility struct {
	ID                     string     `json:"id"`
	EntityProfileID        string     `json:"entity_profile_id"`
	PartnerRef             string     `json:"partner_ref"`
	TotalInteractions      int        `json:"total_interactions"`
	SuccessfulInteractions int        `json:"successful_interactions"`
	CompatibilityScore     *float64   `json:"compatibility_score"`
	LastCalculatedAt       *time.Time `json:"last_calculated_at"`
}
