package diagnose

import (
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"
	"github.com/opendatahub-io/opendatahub-operator/pkg/failureclassifier"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Config drives a diagnostic run.
type Config struct {
	Client       client.Client
	AppsNS       string
	OperatorNS   string
	OperatorName string

	// TargetComponent scopes Correlate to a single component (e.g. "kserve").
	// Empty = correlate all known components.
	TargetComponent string

	EventsSince time.Duration
}

// Report is the structured result of a diagnostic run.
type Report struct {
	Healthy        bool                                     `json:"healthy"`
	Health         *clusterhealth.Report                    `json:"health,omitempty"`
	Classification *failureclassifier.FailureClassification `json:"classification,omitempty"`
	Components     []*clusterhealth.ComponentStatusResult   `json:"components,omitempty"`
	Events         []clusterhealth.EventInfo                `json:"events,omitempty"`
}
