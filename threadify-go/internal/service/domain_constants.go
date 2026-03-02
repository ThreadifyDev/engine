package service

import (
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
