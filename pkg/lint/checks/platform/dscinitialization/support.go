package dscinitialization

import (
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// Condition types for optional observability capabilities, mirrored from the Monitoring CR
// by the DSCI controller, and reasons for overall monitoring readiness (see the
// operator's internal/controller/status/status.go and dscinitialization controller).
const (
	conditionTypeMonitoringReady = "MonitoringReady"
	reasonRemoved                = "Removed"

	// Metrics-dependent capabilities.
	conditionTypeMonitoringStackAvailable     = "MonitoringStackAvailable"
	conditionTypeThanosQuerierAvailable       = "ThanosQuerierAvailable"
	conditionTypeNodeMetricsEndpointAvailable = "NodeMetricsEndpointAvailable"
	conditionTypePersesPrometheusDSAvailable  = "PersesPrometheusDataSourceAvailable"

	// Traces-dependent capabilities.
	conditionTypeTempoAvailable         = "TempoAvailable"
	conditionTypePersesTempoDSAvailable = "PersesTempoDataSourceAvailable"

	// Capabilities requiring at least one of metrics or traces.
	conditionTypeOTelCollectorAvailable = "OpenTelemetryCollectorAvailable"
	conditionTypePersesAvailable        = "PersesAvailable"

	// Alerting capability.
	conditionTypeAlertingAvailable = "AlertingAvailable"
)

// monitoringNotManagedExpr matches a DSCI spec where monitoring is not Managed (empty or
// Removed): the operator deletes the Monitoring CR, making every capability condition
// expected regardless of spec feature fields.
const (
	monitoringNotManagedExpr = `.spec.monitoring.managementState != "Managed"`
)

// Feature-unconfigured expressions. The operator treats metrics/traces as configured only
// when storage or at least one exporter is set: bare feature blocks are projected away,
// yielding NotConfigured conditions. Exporters-only configurations count as configured.
const (
	metricsUnconfiguredExpr = `.spec.monitoring.metrics.storage == null and (.spec.monitoring.metrics.exporters | length) == 0`
	tracesUnconfiguredExpr  = `.spec.monitoring.traces.storage == null and (.spec.monitoring.traces.exporters | length) == 0`
)

// isExpectedUnconfiguredMonitoringFalse returns true when a False condition on an optional
// observability capability is expected because the DSCI spec does not configure the
// capability or monitoring is disabled entirely. The operator sets phase Ready alongside
// such conditions, so they are healthy states.
//
// When monitoring is not Managed, the operator deletes the Monitoring CR entirely, making
// every capability condition expected regardless of spec feature fields, as is
// MonitoringReady=False with reason Removed ("Monitoring is not enabled"). When monitoring
// is Managed, the spec must not configure the capability: a False condition on a
// configured capability indicates a genuine reconcile failure and stays blocking. Any
// other condition type is expected to be True.
func isExpectedUnconfiguredMonitoringFalse(cond metav1.Condition, obj unstructured.Unstructured) bool { //nolint:cyclop // One case per condition type with an explicit body is intentional.
	if cond.Status != metav1.ConditionFalse {
		return false
	}

	//nolint:revive // identical-switch-branches: one case per condition type keeps the mapping explicit.
	switch cond.Type {
	// Metrics-dependent capabilities.
	case conditionTypeMonitoringStackAvailable:
		return jq.Matches(obj, monitoringNotManagedExpr) || jq.Matches(obj, metricsUnconfiguredExpr)
	case conditionTypeThanosQuerierAvailable:
		return jq.Matches(obj, monitoringNotManagedExpr) || jq.Matches(obj, metricsUnconfiguredExpr)
	case conditionTypeNodeMetricsEndpointAvailable:
		return jq.Matches(obj, monitoringNotManagedExpr) || jq.Matches(obj, metricsUnconfiguredExpr)
	case conditionTypePersesPrometheusDSAvailable:
		return jq.Matches(obj, monitoringNotManagedExpr) || jq.Matches(obj, metricsUnconfiguredExpr)
	// Traces-dependent capabilities.
	case conditionTypeTempoAvailable:
		return jq.Matches(obj, monitoringNotManagedExpr) || jq.Matches(obj, tracesUnconfiguredExpr)
	case conditionTypePersesTempoDSAvailable:
		return jq.Matches(obj, monitoringNotManagedExpr) || jq.Matches(obj, tracesUnconfiguredExpr)
	// Capabilities requiring at least one of metrics or traces.
	case conditionTypeOTelCollectorAvailable:
		return jq.Matches(obj, monitoringNotManagedExpr) ||
			(jq.Matches(obj, metricsUnconfiguredExpr) && jq.Matches(obj, tracesUnconfiguredExpr))
	case conditionTypePersesAvailable:
		return jq.Matches(obj, monitoringNotManagedExpr) ||
			(jq.Matches(obj, metricsUnconfiguredExpr) && jq.Matches(obj, tracesUnconfiguredExpr))
	// Alerting capability.
	case conditionTypeAlertingAvailable:
		return jq.Matches(obj, monitoringNotManagedExpr) || jq.Matches(obj, `.spec.monitoring.alerting == null`)
	case conditionTypeMonitoringReady:
		return cond.Reason == reasonRemoved && jq.Matches(obj, monitoringNotManagedExpr)
	default:
		return false
	}
}

// collectUnhappyDSCIConditionMessages returns messages from DSCI status conditions that
// deviate from their expected state. Progressing and Degraded conditions are expected to
// be False when healthy; all other condition types are expected to be True.
//
// ServiceMesh capability conditions receive special handling:
//   - CapabilityServiceMesh: False with reason Removed or Unmanaged is a healthy state.
//   - CapabilityServiceMeshAuthorization: only False with reason NotReady is a genuine
//     problem; all other False reasons (Removed, Unmanaged, MissingOperator) are non-blocking.
//
// Observability capability conditions receive special handling: a False condition on an
// optional monitoring capability is healthy when the DSCI spec does not configure that
// capability or monitoring is disabled entirely.
//
// Returns nil if no conditions field exists or no conditions are unhappy.
func collectUnhappyDSCIConditionMessages(obj *unstructured.Unstructured) ([]string, error) {
	conditions, err := jq.Query[[]metav1.Condition](obj, ".status.conditions")
	if errors.Is(err, jq.ErrNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("querying DSCI conditions: %w", err)
	}

	var messages []string

	for _, cond := range conditions {
		// Progressing and Degraded follow negative semantics: only False is healthy.
		// All other conditions expect True. Any value other than the expected one is unhappy.
		expectFalse := cond.Type == "Progressing" || cond.Type == "Degraded"
		unhappy := (expectFalse && cond.Status != metav1.ConditionFalse) ||
			(!expectFalse && cond.Status != metav1.ConditionTrue)

		if isExpectedCapabilityServiceMeshFalse(cond) ||
			isExpectedCapabilityServiceMeshAuthFalse(cond) ||
			isExpectedUnconfiguredMonitoringFalse(cond, *obj) {
			unhappy = false
		}

		if unhappy && cond.Message != "" {
			messages = append(messages, fmt.Sprintf("%s: %s", cond.Type, cond.Message))
		}
	}

	return messages, nil
}

// isExpectedCapabilityServiceMeshFalse returns true when the CapabilityServiceMesh condition
// is False because the capability was explicitly removed or set to unmanaged, both of which
// are intentional user choices and expected healthy states.
func isExpectedCapabilityServiceMeshFalse(cond metav1.Condition) bool {
	return cond.Type == "CapabilityServiceMesh" &&
		cond.Status == metav1.ConditionFalse &&
		(cond.Reason == reasonRemoved || cond.Reason == "Unmanaged")
}

// isExpectedCapabilityServiceMeshAuthFalse returns true when the CapabilityServiceMeshAuthorization
// condition is False for a non-blocking reason. Only NotReady indicates a genuine problem; all
// other False reasons (Removed, Unmanaged, MissingOperator) represent intentional or environmental
// states that are not upgrade blockers.
//
// A denylist (reason != NotReady) is used intentionally over an explicit allowlist so that new
// benign reasons added upstream are automatically treated as non-blocking without a code change.
func isExpectedCapabilityServiceMeshAuthFalse(cond metav1.Condition) bool {
	return cond.Type == "CapabilityServiceMeshAuthorization" &&
		cond.Status == metav1.ConditionFalse &&
		cond.Reason != "NotReady"
}

// validatePhaseReady checks that the status.phase field is "Ready" on a resource and there are no unexpected conditions.
func validatePhaseReady(
	dr *result.DiagnosticResult,
	obj *unstructured.Unstructured,
	resourceName string,
) error {
	phaseStatus, err := jq.Query[string](obj, `.status.phase`)

	switch {
	case errors.Is(err, jq.ErrNotFound):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("%s resource found but phase field is missing", resourceName),
			check.WithImpact(result.ImpactBlocking),
		))
	case err != nil:
		return fmt.Errorf("querying %s phase: %w", resourceName, err)
	case phaseStatus == "":
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("%s resource found but phase is empty", resourceName),
			check.WithImpact(result.ImpactBlocking),
		))
	case phaseStatus != "Ready":
		condMessages, msgErr := collectUnhappyDSCIConditionMessages(obj)
		if msgErr != nil {
			return fmt.Errorf("collecting conditions from %s: %w", resourceName, msgErr)
		}

		msg := fmt.Sprintf("%s is not ready (phase: %s). %s must be ready before upgrading", resourceName, phaseStatus, resourceName)
		if len(condMessages) > 0 {
			msg += ". Conditions: " + strings.Join(condMessages, "; ")
		}

		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceUnavailable),
			check.WithMessage("%s", msg),
			check.WithImpact(result.ImpactBlocking),
		))
	default:
		// Phase is Ready, but conditions may still signal problems.
		condMessages, msgErr := collectUnhappyDSCIConditionMessages(obj)
		if msgErr != nil {
			return fmt.Errorf("collecting conditions from %s: %w", resourceName, msgErr)
		}

		if len(condMessages) > 0 {
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeReady,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonResourceUnavailable),
				check.WithMessage("%s phase is Ready but some conditions are not expected. Conditions: %s", resourceName, strings.Join(condMessages, "; ")),
				check.WithImpact(result.ImpactBlocking),
			))
		} else {
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeReady,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonResourceAvailable),
				check.WithMessage("%s is ready", resourceName),
			))
		}
	}

	return nil
}
