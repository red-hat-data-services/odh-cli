package conditions

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FindCondition finds a condition by type from a list of conditions.
// Returns nil if not found.
func FindCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}

	return nil
}

// FindReady finds the Ready condition from a list of conditions.
// Returns nil if not found.
func FindReady(conditions []metav1.Condition) *metav1.Condition {
	return FindCondition(conditions, "Ready")
}

// IsTrue checks if a condition type is present and has status True.
func IsTrue(conditions []metav1.Condition, conditionType string) bool {
	cond := FindCondition(conditions, conditionType)

	return cond != nil && cond.Status == metav1.ConditionTrue
}

// IsFalse checks if a condition type is present and has status False.
func IsFalse(conditions []metav1.Condition, conditionType string) bool {
	cond := FindCondition(conditions, conditionType)

	return cond != nil && cond.Status == metav1.ConditionFalse
}

// CollectMessages collects messages from conditions matching the given types
// that have status True. Returns a semicolon-separated string.
func CollectMessages(conditions []metav1.Condition, conditionTypes ...string) string {
	var messages []string

	typeSet := make(map[string]struct{}, len(conditionTypes))
	for _, t := range conditionTypes {
		typeSet[t] = struct{}{}
	}

	for _, c := range conditions {
		if _, ok := typeSet[c.Type]; ok {
			if c.Status == metav1.ConditionTrue && c.Message != "" {
				messages = append(messages, c.Message)
			}
		}
	}

	return strings.Join(messages, "; ")
}

// CollectDegradedMessages collects messages from Degraded conditions that are True.
func CollectDegradedMessages(conditions []metav1.Condition) string {
	return CollectMessages(conditions, "Degraded")
}
