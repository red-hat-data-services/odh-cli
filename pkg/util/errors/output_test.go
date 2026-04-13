package errors_test

import (
	"bytes"
	"errors"
	"testing"

	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"

	. "github.com/onsi/gomega"
)

func TestWriteTextError(t *testing.T) {
	t.Run("should render message and suggestion", func(t *testing.T) {
		g := NewWithT(t)
		buf := &bytes.Buffer{}
		err := &clierrors.StructuredError{
			Code:       "AUTH_FAILED",
			Message:    "token expired",
			Category:   clierrors.CategoryAuthentication,
			Suggestion: "Refresh credentials",
		}

		handled := clierrors.WriteTextError(buf, err)

		g.Expect(handled).To(BeTrue())
		g.Expect(buf.String()).To(ContainSubstring("token expired"))
		g.Expect(buf.String()).To(ContainSubstring("Suggestion: Refresh credentials"))
	})

	t.Run("should auto-classify raw errors", func(t *testing.T) {
		g := NewWithT(t)
		buf := &bytes.Buffer{}
		rawErr := errors.New("something broke")

		handled := clierrors.WriteTextError(buf, rawErr)

		g.Expect(handled).To(BeTrue())
		g.Expect(buf.String()).To(ContainSubstring("something broke"))
		g.Expect(buf.String()).To(ContainSubstring("Suggestion:"))
	})

	t.Run("should return false for nil error", func(t *testing.T) {
		g := NewWithT(t)
		buf := &bytes.Buffer{}

		handled := clierrors.WriteTextError(buf, nil)

		g.Expect(handled).To(BeFalse())
	})
}

func TestWriteStructuredError(t *testing.T) {
	t.Run("should render JSON output", func(t *testing.T) {
		g := NewWithT(t)
		buf := &bytes.Buffer{}
		err := &clierrors.StructuredError{
			Code:       "AUTH_FAILED",
			Message:    "token expired",
			Category:   clierrors.CategoryAuthentication,
			Retriable:  false,
			Suggestion: "Refresh credentials",
		}

		handled := clierrors.WriteStructuredError(buf, err, "json")

		g.Expect(handled).To(BeTrue())
		g.Expect(buf.String()).To(ContainSubstring(`"code": "AUTH_FAILED"`))
		g.Expect(buf.String()).To(ContainSubstring(`"category": "authentication"`))
		g.Expect(buf.String()).To(ContainSubstring(`"retriable": false`))
		g.Expect(buf.String()).To(ContainSubstring(`"suggestion": "Refresh credentials"`))
	})

	t.Run("should render YAML output", func(t *testing.T) {
		g := NewWithT(t)
		buf := &bytes.Buffer{}
		err := &clierrors.StructuredError{
			Code:       "AUTH_FAILED",
			Message:    "token expired",
			Category:   clierrors.CategoryAuthentication,
			Retriable:  false,
			Suggestion: "Refresh credentials",
		}

		handled := clierrors.WriteStructuredError(buf, err, "yaml")

		g.Expect(handled).To(BeTrue())
		g.Expect(buf.String()).To(ContainSubstring("code: AUTH_FAILED"))
		g.Expect(buf.String()).To(ContainSubstring("category: authentication"))
	})

	t.Run("should not render table output", func(t *testing.T) {
		g := NewWithT(t)
		buf := &bytes.Buffer{}
		err := &clierrors.StructuredError{
			Code:     "AUTH_FAILED",
			Message:  "token expired",
			Category: clierrors.CategoryAuthentication,
		}

		handled := clierrors.WriteStructuredError(buf, err, "table")

		g.Expect(handled).To(BeFalse())
		g.Expect(buf.String()).To(BeEmpty())
	})

	t.Run("should return false for nil error", func(t *testing.T) {
		g := NewWithT(t)
		buf := &bytes.Buffer{}

		handled := clierrors.WriteStructuredError(buf, nil, "json")

		g.Expect(handled).To(BeFalse())
	})

	t.Run("should auto-classify raw errors", func(t *testing.T) {
		g := NewWithT(t)
		buf := &bytes.Buffer{}
		rawErr := errors.New("something broke")

		handled := clierrors.WriteStructuredError(buf, rawErr, "json")

		g.Expect(handled).To(BeTrue())
		g.Expect(buf.String()).To(ContainSubstring(`"category": "internal"`))
		g.Expect(buf.String()).To(ContainSubstring(`"code": "INTERNAL"`))
	})
}
