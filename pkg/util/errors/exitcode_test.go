package errors_test

import (
	"errors"
	"fmt"
	"testing"

	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"

	. "github.com/onsi/gomega"
)

const (
	testMsgAdvisory       = "advisory"
	testMsgForbidden      = "forbidden"
	testMsgTokenExpired   = "token expired"
	testMsgSomethingBroke = "something broke"
	testMsgAdvisoryFound  = "advisory found"
	testMsgOriginalError  = "original error"
	testMsgBadInput       = "bad input"
	testMsgTimeout        = "timeout"
	testCodeAuthFailed    = "AUTH_FAILED"
	testUnknownCategory   = "unknown_category"
)

func TestExitCodeFromCategory(t *testing.T) {
	cases := []struct {
		name     string
		category clierrors.ErrorCategory
		want     clierrors.ExitCode
	}{
		{"authentication maps to ExitAuth", clierrors.CategoryAuthentication, clierrors.ExitAuth},
		{"authorization maps to ExitAuth", clierrors.CategoryAuthorization, clierrors.ExitAuth},
		{"connection maps to ExitConnection", clierrors.CategoryConnection, clierrors.ExitConnection},
		{"timeout maps to ExitConnection", clierrors.CategoryTimeout, clierrors.ExitConnection},
		{"validation maps to ExitValidation", clierrors.CategoryValidation, clierrors.ExitValidation},
		{"server maps to ExitError", clierrors.CategoryServer, clierrors.ExitError},
		{"not_found maps to ExitError", clierrors.CategoryNotFound, clierrors.ExitError},
		{"conflict maps to ExitError", clierrors.CategoryConflict, clierrors.ExitError},
		{"internal maps to ExitError", clierrors.CategoryInternal, clierrors.ExitError},
		{"unknown category maps to ExitError", testUnknownCategory, clierrors.ExitError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(clierrors.ExitCodeFromCategory(tc.category)).To(Equal(tc.want))
		})
	}
}

func TestExitCodeFromError(t *testing.T) {
	structErrAuth := &clierrors.StructuredError{
		Code:     testCodeAuthFailed,
		Message:  testMsgTokenExpired,
		Category: clierrors.CategoryAuthentication,
	}

	cases := []struct {
		name string
		err  error
		want clierrors.ExitCode
	}{
		{
			name: "nil error returns ExitSuccess",
			err:  nil,
			want: clierrors.ExitSuccess,
		},
		{
			name: "ExitCodeError returns its code",
			err:  clierrors.NewExitCodeError(clierrors.ExitWarning, errors.New(testMsgAdvisory)),
			want: clierrors.ExitWarning,
		},
		{
			name: "wrapped ExitCodeError returns its code",
			err:  fmt.Errorf("outer: %w", clierrors.NewExitCodeError(clierrors.ExitAuth, errors.New(testMsgForbidden))),
			want: clierrors.ExitAuth,
		},
		{
			name: "StructuredError maps via category",
			err:  structErrAuth,
			want: clierrors.ExitAuth,
		},
		{
			name: "StructuredError with explicit ExitCode uses that code",
			err: &clierrors.StructuredError{
				Category: clierrors.CategoryInternal,
				ExitCode: int(clierrors.ExitWarning),
				Message:  "explicit exit code takes precedence",
			},
			want: clierrors.ExitWarning,
		},
		{
			name: "plain error classifies as ExitError",
			err:  errors.New(testMsgSomethingBroke),
			want: clierrors.ExitError,
		},
		{
			name: "ExitCodeError takes priority over StructuredError in chain",
			err:  clierrors.NewExitCodeError(clierrors.ExitConnection, structErrAuth),
			want: clierrors.ExitConnection,
		},
		{
			name: "AlreadyHandledError wrapping plain error returns ExitError",
			err:  clierrors.NewAlreadyHandledError(errors.New(testMsgSomethingBroke)),
			want: clierrors.ExitError,
		},
		{
			name: "AlreadyHandledError preserves inner ExitCodeError code",
			err:  clierrors.NewAlreadyHandledError(clierrors.NewExitCodeError(clierrors.ExitWarning, errors.New(testMsgAdvisoryFound))),
			want: clierrors.ExitWarning,
		},
		{
			name: "double-wrapped ExitCodeError uses outermost code",
			err:  clierrors.NewExitCodeError(clierrors.ExitConnection, clierrors.NewExitCodeError(clierrors.ExitWarning, errors.New(testMsgAdvisoryFound))),
			want: clierrors.ExitConnection,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(clierrors.ExitCodeFromError(tc.err)).To(Equal(tc.want))
		})
	}
}

func TestIsHigherPriority(t *testing.T) {
	pairCases := []struct {
		name string
		a    clierrors.ExitCode
		b    clierrors.ExitCode
		want bool
	}{
		{"connection beats auth", clierrors.ExitConnection, clierrors.ExitAuth, true},
		{"auth beats validation", clierrors.ExitAuth, clierrors.ExitValidation, true},
		{"validation beats error", clierrors.ExitValidation, clierrors.ExitError, true},
		{"error beats warning", clierrors.ExitError, clierrors.ExitWarning, true},
		{"warning beats success", clierrors.ExitWarning, clierrors.ExitSuccess, true},
		{"same code is not strictly higher", clierrors.ExitAuth, clierrors.ExitAuth, false},
		{"lower does not beat higher", clierrors.ExitWarning, clierrors.ExitConnection, false},
		{"success does not beat error", clierrors.ExitSuccess, clierrors.ExitError, false},
	}

	for _, tc := range pairCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(clierrors.IsHigherPriority(tc.a, tc.b)).To(Equal(tc.want))
		})
	}

	t.Run("unknown code is treated as at least error priority", func(t *testing.T) {
		g := NewWithT(t)
		unknown := clierrors.ExitCode(99)

		g.Expect(clierrors.IsHigherPriority(unknown, clierrors.ExitSuccess)).To(BeTrue())
		g.Expect(clierrors.IsHigherPriority(unknown, clierrors.ExitWarning)).To(BeTrue())
	})

	t.Run("known high-priority code still beats unknown", func(t *testing.T) {
		g := NewWithT(t)
		unknown := clierrors.ExitCode(99)

		g.Expect(clierrors.IsHigherPriority(clierrors.ExitConnection, unknown)).To(BeTrue())
		g.Expect(clierrors.IsHigherPriority(clierrors.ExitAuth, unknown)).To(BeTrue())
		g.Expect(clierrors.IsHigherPriority(clierrors.ExitValidation, unknown)).To(BeTrue())
	})

	t.Run("all defined exit codes beat ExitSuccess", func(t *testing.T) {
		g := NewWithT(t)
		codes := []clierrors.ExitCode{
			clierrors.ExitError,
			clierrors.ExitWarning,
			clierrors.ExitValidation,
			clierrors.ExitAuth,
			clierrors.ExitConnection,
		}
		for _, code := range codes {
			g.Expect(clierrors.IsHigherPriority(code, clierrors.ExitSuccess)).To(BeTrue())
		}
	})
}

func TestExitCodeError(t *testing.T) {
	t.Run("should implement error interface", func(t *testing.T) {
		g := NewWithT(t)
		err := clierrors.NewExitCodeError(clierrors.ExitWarning, errors.New(testMsgAdvisoryFound))
		g.Expect(err.Error()).To(Equal(testMsgAdvisoryFound))
	})

	t.Run("should unwrap to original error", func(t *testing.T) {
		g := NewWithT(t)
		original := errors.New(testMsgOriginalError)
		err := clierrors.NewExitCodeError(clierrors.ExitAuth, original)
		g.Expect(errors.Is(err, original)).To(BeTrue())
	})

	t.Run("should preserve exit code", func(t *testing.T) {
		g := NewWithT(t)
		err := clierrors.NewExitCodeError(clierrors.ExitValidation, errors.New(testMsgBadInput))

		var exitErr *clierrors.ExitCodeError
		g.Expect(errors.As(err, &exitErr)).To(BeTrue())
		g.Expect(exitErr.Code).To(Equal(clierrors.ExitValidation))
	})

	t.Run("should be extractable via errors.As", func(t *testing.T) {
		g := NewWithT(t)
		inner := clierrors.NewExitCodeError(clierrors.ExitConnection, errors.New(testMsgTimeout))
		wrapped := fmt.Errorf("wrapper: %w", inner)

		var exitErr *clierrors.ExitCodeError
		g.Expect(errors.As(wrapped, &exitErr)).To(BeTrue())
		g.Expect(exitErr.Code).To(Equal(clierrors.ExitConnection))
	})
}
