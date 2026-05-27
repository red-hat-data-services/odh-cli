package action_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"

	. "github.com/onsi/gomega"
)

func TestActionPhase_Validate(t *testing.T) {
	t.Run("should accept pre-upgrade", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(action.PhasePreUpgrade.Validate()).ToNot(HaveOccurred())
	})

	t.Run("should accept post-upgrade", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(action.PhasePostUpgrade.Validate()).ToNot(HaveOccurred())
	})

	t.Run("should accept pre-enablement", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(action.PhasePreEnablement.Validate()).ToNot(HaveOccurred())
	})

	t.Run("should accept empty string", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(action.ActionPhase("").Validate()).ToNot(HaveOccurred())
	})

	t.Run("should reject invalid phase", func(t *testing.T) {
		g := NewWithT(t)
		err := action.ActionPhase("invalid").Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid phase"))
	})
}
