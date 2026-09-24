package domain

// CanDecision explains whether an authored contract action is eligible now.
// It is a read-only snapshot; execution still requires an atomic waitFor claim.
type CanDecision struct {
	ThreadID       string
	StepName       string
	Allowed        bool
	Status         string
	MatchedBy      string
	RequiredSteps  []string
	SatisfiedSteps []string
	MissingSteps   []string
	PreviousStep   *string
	Reason         string
}

// ShouldDecision is advisory and never grants execution permission.
type ShouldDecision struct {
	ThreadID       string
	StepName       string
	Eligible       bool
	Recommendation string // yes, no, uncertain, or unavailable
	Reason         string
}

// NextPath starts with an eligible action. Later actions are conditional.
type NextPath struct {
	Actions []string
	Status  string // ready or requires_claim
	Reason  string
}

type NextDecision struct {
	ThreadID string
	Paths    []*NextPath
}
