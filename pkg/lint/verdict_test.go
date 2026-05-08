//nolint:testpackage // internal test: exercises unexported evaluateVerdict method
package lint

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"

	. "github.com/onsi/gomega"
)

const (
	testVerdictGroup       = "test"
	testVerdictKind        = "resource"
	testVerdictCheckName   = "check"
	testVerdictDescription = "test check"
)

func newTestCommand() *Command {
	var out, errOut bytes.Buffer
	streams := genericiooptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &out,
		ErrOut: &errOut,
	}

	return NewCommand(streams, genericclioptions.NewConfigFlags(true))
}

func buildExecution(impact result.Impact) check.CheckExecution {
	dr := result.New(testVerdictGroup, testVerdictKind, testVerdictCheckName, testVerdictDescription)
	dr.SetCondition(check.NewCondition(
		check.ConditionTypeAvailable,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonResourceNotFound),
		check.WithMessage("test finding"),
		check.WithImpact(impact),
	))

	return check.CheckExecution{Result: dr}
}

func buildPassingExecution() check.CheckExecution {
	dr := result.New(testVerdictGroup, testVerdictKind, testVerdictCheckName, testVerdictDescription)
	dr.SetCondition(check.NewCondition(
		check.ConditionTypeAvailable,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonResourceAvailable),
		check.WithMessage("all good"),
	))

	return check.CheckExecution{Result: dr}
}

func TestEvaluateVerdict(t *testing.T) {
	cases := []struct {
		name              string
		results           []check.CheckExecution
		wantErr           bool
		wantCode          clierrors.ExitCode
		notAlreadyHandled bool
	}{
		{
			name:    "should return nil for all-passing results",
			results: []check.CheckExecution{buildPassingExecution()},
		},
		{
			name:     "should return ExitError for prohibited findings",
			results:  []check.CheckExecution{buildExecution(result.ImpactProhibited)},
			wantErr:  true,
			wantCode: clierrors.ExitError,
		},
		{
			name:     "should return ExitError for blocking findings",
			results:  []check.CheckExecution{buildExecution(result.ImpactBlocking)},
			wantErr:  true,
			wantCode: clierrors.ExitError,
		},
		{
			name:     "should return ExitWarning for advisory-only findings",
			results:  []check.CheckExecution{buildExecution(result.ImpactAdvisory)},
			wantErr:  true,
			wantCode: clierrors.ExitWarning,
		},
		{
			name: "should return ExitError when both prohibited and advisory findings exist",
			results: []check.CheckExecution{
				buildExecution(result.ImpactProhibited),
				buildExecution(result.ImpactAdvisory),
			},
			wantErr:           true,
			wantCode:          clierrors.ExitError,
			notAlreadyHandled: true,
		},
		{
			name: "should return ExitError when both blocking and advisory findings exist",
			results: []check.CheckExecution{
				buildExecution(result.ImpactBlocking),
				buildExecution(result.ImpactAdvisory),
			},
			wantErr:  true,
			wantCode: clierrors.ExitError,
		},
		{
			name: "should skip nil results",
			results: []check.CheckExecution{
				{Result: nil},
				buildExecution(result.ImpactAdvisory),
			},
			wantErr:  true,
			wantCode: clierrors.ExitWarning,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			cmd := newTestCommand()
			cmd.OutputFormat = OutputFormatJSON

			err := cmd.evaluateVerdict(tc.results)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(clierrors.ExitCodeFromError(err)).To(Equal(tc.wantCode))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tc.notAlreadyHandled {
				g.Expect(errors.Is(err, clierrors.ErrAlreadyHandled)).To(BeFalse())
			}
		})
	}
}

func buildExecutionWithError(execErr error) check.CheckExecution {
	return check.CheckExecution{
		Result: nil,
		Error:  execErr,
	}
}

func TestHighestPriorityExecError(t *testing.T) {
	connErr := clierrors.NewExitCodeError(clierrors.ExitConnection, errors.New("connection failed"))
	authErr := clierrors.NewExitCodeError(clierrors.ExitAuth, errors.New("auth failed"))
	genericErr := errors.New("generic error")

	cases := []struct {
		name     string
		results  []check.CheckExecution
		wantCode clierrors.ExitCode
		wantErr  bool
	}{
		{
			name:     "should return ExitSuccess for no errors",
			results:  []check.CheckExecution{buildPassingExecution()},
			wantCode: clierrors.ExitSuccess,
			wantErr:  false,
		},
		{
			name:     "should return connection error code",
			results:  []check.CheckExecution{buildExecutionWithError(connErr)},
			wantCode: clierrors.ExitConnection,
			wantErr:  true,
		},
		{
			name:     "should return auth error code",
			results:  []check.CheckExecution{buildExecutionWithError(authErr)},
			wantCode: clierrors.ExitAuth,
			wantErr:  true,
		},
		{
			name: "should return highest priority when multiple errors exist",
			results: []check.CheckExecution{
				buildExecutionWithError(authErr),
				buildExecutionWithError(connErr),
			},
			wantCode: clierrors.ExitConnection,
			wantErr:  true,
		},
		{
			name: "should classify generic error as ExitError",
			results: []check.CheckExecution{
				buildExecutionWithError(genericErr),
			},
			wantCode: clierrors.ExitError,
			wantErr:  true,
		},
		{
			name: "should ignore nil errors in results",
			results: []check.CheckExecution{
				{Result: nil, Error: nil},
				buildExecutionWithError(authErr),
			},
			wantCode: clierrors.ExitAuth,
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			summary := highestPriorityExecError(tc.results)

			g.Expect(summary.exitCode).To(Equal(tc.wantCode))
			if tc.wantErr {
				g.Expect(summary.err).To(HaveOccurred())
			} else {
				g.Expect(summary.err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestResolveExitError(t *testing.T) {
	connErr := errors.New("connection failed")
	findingsErr := clierrors.NewExitCodeError(clierrors.ExitError,
		fmt.Errorf("%w: blocked", clierrors.ErrLintBlocked))
	advisoryErr := clierrors.NewExitCodeError(clierrors.ExitWarning,
		fmt.Errorf("%w: advisory", clierrors.ErrLintAdvisory))

	cases := []struct {
		name         string
		execSummary  execErrorSummary
		findingsErr  error
		outputFormat OutputFormat
		wantCode     clierrors.ExitCode
		wantErr      bool
	}{
		{
			name:         "should return nil when no errors",
			execSummary:  execErrorSummary{exitCode: clierrors.ExitSuccess},
			findingsErr:  nil,
			outputFormat: OutputFormatJSON,
			wantErr:      false,
		},
		{
			name:         "should return findings error when no exec errors",
			execSummary:  execErrorSummary{exitCode: clierrors.ExitSuccess},
			findingsErr:  findingsErr,
			outputFormat: OutputFormatJSON,
			wantCode:     clierrors.ExitError,
			wantErr:      true,
		},
		{
			name:         "should prefer exec error when higher priority than findings",
			execSummary:  execErrorSummary{exitCode: clierrors.ExitConnection, err: connErr},
			findingsErr:  findingsErr,
			outputFormat: OutputFormatJSON,
			wantCode:     clierrors.ExitConnection,
			wantErr:      true,
		},
		{
			name:         "should prefer exec error when equal priority to findings",
			execSummary:  execErrorSummary{exitCode: clierrors.ExitError, err: connErr},
			findingsErr:  findingsErr,
			outputFormat: OutputFormatJSON,
			wantCode:     clierrors.ExitError,
			wantErr:      true,
		},
		{
			name:         "should return findings when higher priority than exec error",
			execSummary:  execErrorSummary{exitCode: clierrors.ExitWarning, err: connErr},
			findingsErr:  findingsErr,
			outputFormat: OutputFormatJSON,
			wantCode:     clierrors.ExitError,
			wantErr:      true,
		},
		{
			name:         "should wrap as AlreadyHandled for table format",
			execSummary:  execErrorSummary{exitCode: clierrors.ExitSuccess},
			findingsErr:  advisoryErr,
			outputFormat: OutputFormatTable,
			wantCode:     clierrors.ExitWarning,
			wantErr:      true,
		},
		{
			name:         "should return exec error when no findings",
			execSummary:  execErrorSummary{exitCode: clierrors.ExitConnection, err: connErr},
			findingsErr:  nil,
			outputFormat: OutputFormatJSON,
			wantCode:     clierrors.ExitConnection,
			wantErr:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			err := resolveExitError(tc.execSummary, tc.findingsErr, tc.outputFormat)

			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(clierrors.ExitCodeFromError(err)).To(Equal(tc.wantCode))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}

	t.Run("should wrap as AlreadyHandled for table format with findings", func(t *testing.T) {
		g := NewWithT(t)
		err := resolveExitError(
			execErrorSummary{exitCode: clierrors.ExitSuccess},
			advisoryErr,
			OutputFormatTable,
		)

		g.Expect(errors.Is(err, clierrors.ErrAlreadyHandled)).To(BeTrue())
	})
}
