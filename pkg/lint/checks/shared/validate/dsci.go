package validate

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
)

// DSCIBuilder provides a fluent API for DSCInitialization-based validation.
// It handles DSCI fetching and annotation population automatically.
type DSCIBuilder struct {
	check check.Check
}

// DSCI creates a builder for DSCInitialization-based validation.
// This is used by service checks that need to read platform configuration from DSCI.
//
// Example:
//
//	validate.DSCI(c).
//	    Run(ctx, target, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
//	        // Validation logic here
//	        return nil
//	    })
func DSCI(c check.Check) *DSCIBuilder {
	return &DSCIBuilder{check: c}
}

// DSCIValidateFn is the validation function called after DSCI is fetched.
// It receives an auto-created DiagnosticResult with pre-populated annotations and the fetched DSCI.
type DSCIValidateFn func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error

// Run fetches the DSCI, auto-populates annotations, and executes validation.
//
// The builder handles:
//   - DSCI not found: returns a standard "not found" diagnostic result (not an error)
//   - DSCI fetch error: returns wrapped error
//   - Annotation population: target version is automatically added
//
// Returns (*result.DiagnosticResult, error) following the standard lint check signature.
func (b *DSCIBuilder) Run(
	ctx context.Context,
	target check.Target,
	fn DSCIValidateFn,
) (*result.DiagnosticResult, error) {
	// Fetch the DSCInitialization singleton
	dsci, err := client.GetDSCInitialization(ctx, target.Client)
	switch {
	case apierrors.IsNotFound(err):
		return results.DSCInitializationNotFound(
			string(b.check.Group()),
			b.check.CheckKind(),
			b.check.CheckType(),
			b.check.Description(),
		), nil
	case err != nil:
		return nil, fmt.Errorf("getting DSCInitialization: %w", err)
	}

	// Create result with auto-populated annotations
	dr := result.New(
		string(b.check.Group()),
		b.check.CheckKind(),
		b.check.CheckType(),
		b.check.Description(),
	)

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Execute the validation function
	if err := fn(dr, dsci); err != nil {
		return nil, err
	}

	return dr, nil
}
