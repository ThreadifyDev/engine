package domain

// StepProposal explains whether a step is currently eligible under its
// contract's strict transitions and partial-order prerequisites.
type StepProposal struct {
	ThreadID       string
	StepName       string
	Allowed        bool
	RequiredSteps  []string
	SatisfiedSteps []string
	MissingSteps   []string
	PreviousStep   *string
	Reason         string
}
