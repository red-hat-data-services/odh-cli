package components_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/components"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"

	. "github.com/onsi/gomega"
)

func TestErrComponentNotFound(t *testing.T) {
	g := NewWithT(t)

	available := []string{"dashboard", "kserve", "ray"}
	err := components.ErrComponentNotFound("unknown", available)

	g.Expect(err).To(BeAssignableToTypeOf(&clierrors.StructuredError{}))
	g.Expect(err.Code).To(Equal("COMPONENT_NOT_FOUND"))
	g.Expect(err.Message).To(ContainSubstring("unknown"))
	g.Expect(err.Category).To(Equal(clierrors.CategoryNotFound))
	g.Expect(err.Retriable).To(BeFalse())
	g.Expect(err.Suggestion).To(ContainSubstring("dashboard"))
	g.Expect(err.Suggestion).To(ContainSubstring("kserve"))
	g.Expect(err.Suggestion).To(ContainSubstring("ray"))
}

func TestErrInvalidOutputFormat(t *testing.T) {
	g := NewWithT(t)

	err := components.ErrInvalidOutputFormat("xml")

	g.Expect(err).To(BeAssignableToTypeOf(&clierrors.StructuredError{}))
	g.Expect(err.Code).To(Equal("INVALID_OUTPUT_FORMAT"))
	g.Expect(err.Message).To(ContainSubstring("xml"))
	g.Expect(err.Category).To(Equal(clierrors.CategoryValidation))
	g.Expect(err.Retriable).To(BeFalse())
	g.Expect(err.Suggestion).To(ContainSubstring("table"))
	g.Expect(err.Suggestion).To(ContainSubstring("json"))
	g.Expect(err.Suggestion).To(ContainSubstring("yaml"))
}

func TestErrUserAborted(t *testing.T) {
	g := NewWithT(t)

	err := components.ErrUserAborted()

	g.Expect(err).To(BeAssignableToTypeOf(&clierrors.StructuredError{}))
	g.Expect(err.Code).To(Equal("USER_ABORTED"))
	g.Expect(err.Message).To(ContainSubstring("aborted"))
	g.Expect(err.Category).To(Equal(clierrors.CategoryValidation))
	g.Expect(err.Retriable).To(BeFalse())
	g.Expect(err.Suggestion).To(ContainSubstring("--yes"))
}
