package aipipelines_test

import (
	"testing"

	"github.com/blang/semver/v4"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/aipipelines"

	. "github.com/onsi/gomega"
)

func TestPostUpgradeCheckAction(t *testing.T) {
	t.Run("metadata", func(t *testing.T) {
		g := NewWithT(t)

		a := &aipipelines.PostUpgradeCheckAction{}
		g.Expect(a.ID()).To(Equal("ai-pipelines.post-upgrade-check"))
		g.Expect(a.Name()).To(Equal("AI Pipelines post-upgrade check"))
		g.Expect(a.Group()).To(Equal(action.GroupValidation))
		g.Expect(a.Phase()).To(Equal(action.PhasePostUpgrade))
	})

	t.Run("CanApply returns true for version 2.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &aipipelines.PostUpgradeCheckAction{}
		v2 := semver.MustParse("2.25.0")
		v3 := semver.MustParse("3.0.0")

		g.Expect(a.CanApply(action.Target{CurrentVersion: &v2})).To(BeTrue())
		g.Expect(a.CanApply(action.Target{CurrentVersion: &v3})).To(BeFalse())
	})

	t.Run("Prepare returns nil", func(t *testing.T) {
		g := NewWithT(t)

		a := &aipipelines.PostUpgradeCheckAction{}
		g.Expect(a.Prepare()).To(BeNil())
	})

	t.Run("Run returns non-nil task", func(t *testing.T) {
		g := NewWithT(t)

		a := &aipipelines.PostUpgradeCheckAction{}
		g.Expect(a.Run()).ToNot(BeNil())
	})
}
