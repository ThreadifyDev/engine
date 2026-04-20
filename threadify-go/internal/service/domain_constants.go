package service

import (
	billingmodels "threadify-go/shared/models"

	"github.com/threadify/engine/internal/models"
)

const (
	ActionConnect           = "connect"
	ActionStartThread       = "startThread"
	ActionRecordThreadEvent = "recordThreadEvent"
	ActionInviteParty       = "inviteParty"
	ActionJoinThread        = "joinThread"
	ActionCloseConnection   = "closeConnection"
	ActionAddRefs           = "addRefs"
)

const (
	ThreadStatusCompleted = string(models.ThreadStatusCompleted)
	ThreadStatusCancelled = string(models.ThreadStatusCancelled)
	ThreadStatusActive    = string(models.ThreadStatusActive)
	ThreadStatusFailed    = string(models.ThreadStatusFailed)
)

const (
	StepStatusSuccess    = "success"
	StepStatusFailed     = "failed"
	StepStatusError      = "error"
	StepStatusPending    = "pending"
	StepStatusCompleted  = "completed"
	StepStatusInProgress = "in_progress"
)

const (
	ValidationStatusPassed   = "passed"
	ValidationStatusViolated = "violated"
	ValidationStatusNone     = "none"
)

const (
	ActivityTypeValidationResult = "validation_result"
	ActorServiceRuleEngine       = "rule_engine"
)

const (
	MeterIngress           = billingmodels.MeterIngress
	MeterEgress            = billingmodels.MeterEgress
	MeterContractExecution = billingmodels.MeterContractExecution
	MeterContractVersion   = billingmodels.MeterContractVersion
	MeterSeatCreate        = billingmodels.MeterSeatCreate
	MeterLLMTokenUsage     = billingmodels.MeterLLMTokenUsage

	// Deprecated: Use MeterIngress
	MeterBandwidthIngress = billingmodels.MeterIngress
	// Deprecated: Use MeterEgress
	MeterBandwidthEgress = billingmodels.MeterEgress
	// Deprecated: Use MeterContractExecution
	MeterContractCreate = billingmodels.MeterContractExecution
	// Deprecated: Use MeterContractVersion
	MeterContractVersionCreate = billingmodels.MeterContractVersion
)

var (
	MeterCreditSpend        = billingmodels.MeterCreditSpend
	MeterCreditTopup        = billingmodels.MeterCreditTopup
	MeterCreditTopupRequest = billingmodels.MeterCreditTopupRequest
)

const (
	fieldEventID           = billingmodels.FieldEventID
	fieldCompanyID         = billingmodels.FieldCompanyID
	fieldMeter             = billingmodels.FieldMeter
	fieldAmount            = billingmodels.FieldAmount
	fieldBillingCycleStart = billingmodels.FieldBillingCycleStart
	fieldTimestamp         = billingmodels.FieldTimestamp
)
