package result

import (
	"time"
)

type StepStatus string

const (
	StepPending   StepStatus = "Pending"
	StepRunning   StepStatus = "Running"
	StepCompleted StepStatus = "Completed"
	StepFailed    StepStatus = "Failed"
	StepSkipped   StepStatus = "Skipped"
)

type ActionResult struct {
	Metadata ActionMetadata
	Spec     ActionSpec
	Status   ActionStatus
}

type ActionMetadata struct {
	Group       string
	Kind        string
	Name        string
	Annotations map[string]string
}

type ActionSpec struct {
	Description string
	DryRun      bool
}

type ActionStatus struct {
	Steps     []ActionStep
	Completed bool
	Error     string
}

type ActionStep struct {
	Name        string
	Description string
	Status      StepStatus
	Message     string
	Timestamp   time.Time
	Children    []ActionStep   `json:"children,omitempty" yaml:"children,omitempty"`
	Details     map[string]any `json:"details,omitempty"  yaml:"details,omitempty"`
}

func New(
	group string,
	kind string,
	name string,
	description string,
) *ActionResult {
	return &ActionResult{
		Metadata: ActionMetadata{
			Group:       group,
			Kind:        kind,
			Name:        name,
			Annotations: make(map[string]string),
		},
		Spec: ActionSpec{
			Description: description,
		},
		Status: ActionStatus{
			Steps:     []ActionStep{},
			Completed: false,
		},
	}
}

func (r *ActionResult) HasSkippedSteps() bool {
	return hasSkipped(r.Status.Steps)
}

func hasSkipped(steps []ActionStep) bool {
	for _, s := range steps {
		if s.Status == StepSkipped {
			return true
		}

		if hasSkipped(s.Children) {
			return true
		}
	}

	return false
}

func NewStep(
	name string,
	description string,
	status StepStatus,
	message string,
) ActionStep {
	return ActionStep{
		Name:        name,
		Description: description,
		Status:      status,
		Message:     message,
		Timestamp:   time.Now(),
		Children:    []ActionStep{},
		Details:     make(map[string]any),
	}
}
