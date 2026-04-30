package service

import (
	shareddomain "threadify-go/shared/domain"

	"github.com/threadify/engine/internal/domain"
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
	ThreadStatusCompleted = string(domain.ThreadStatusCompleted)
	ThreadStatusCancelled = string(domain.ThreadStatusCancelled)
	ThreadStatusActive    = string(domain.ThreadStatusActive)
	ThreadStatusFailed    = string(domain.ThreadStatusFailed)
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
	MeterIngress           = shareddomain.MeterIngress
	MeterEgress            = shareddomain.MeterEgress
	MeterContractExecution = shareddomain.MeterContractExecution
	MeterContractVersion   = shareddomain.MeterContractVersion
	MeterSeatCreate        = shareddomain.MeterSeatCreate
	MeterLLMTokenUsage     = shareddomain.MeterLLMTokenUsage
)

var (
	MeterCreditSpend        = shareddomain.MeterCreditSpend
	MeterCreditTopup        = shareddomain.MeterCreditTopup
	MeterCreditTopupRequest = shareddomain.MeterCreditTopupRequest
)

const (
	fieldEventID           = shareddomain.FieldEventID
	fieldCompanyID         = shareddomain.FieldCompanyID
	fieldMeter             = shareddomain.FieldMeter
	fieldAmount            = shareddomain.FieldAmount
	fieldBillingCycleStart = shareddomain.FieldBillingCycleStart
	fieldTimestamp         = shareddomain.FieldTimestamp
)
