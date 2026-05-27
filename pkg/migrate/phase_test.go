package migrate_test

import (
	"testing"

	"github.com/blang/semver/v4"

	"github.com/opendatahub-io/odh-cli/pkg/migrate"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"

	. "github.com/onsi/gomega"
)

func TestDetectPhase(t *testing.T) {
	t.Run("should return pre-upgrade when current < target", func(t *testing.T) {
		g := NewWithT(t)

		current := semver.MustParse("2.25.0")
		target := semver.MustParse("3.0.0")

		phase, err := migrate.DetectPhase(&current, &target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(phase).To(Equal(action.PhasePreUpgrade))
	})

	t.Run("should return post-upgrade when current == target", func(t *testing.T) {
		g := NewWithT(t)

		current := semver.MustParse("3.0.0")
		target := semver.MustParse("3.0.0")

		phase, err := migrate.DetectPhase(&current, &target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(phase).To(Equal(action.PhasePostUpgrade))
	})

	t.Run("should return post-upgrade when current > target", func(t *testing.T) {
		g := NewWithT(t)

		current := semver.MustParse("3.5.0")
		target := semver.MustParse("3.0.0")

		phase, err := migrate.DetectPhase(&current, &target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(phase).To(Equal(action.PhasePostUpgrade))
	})

	t.Run("should return error when current is nil", func(t *testing.T) {
		g := NewWithT(t)

		target := semver.MustParse("3.0.0")

		_, err := migrate.DetectPhase(nil, &target)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("current version"))
	})

	t.Run("should return error when target is nil", func(t *testing.T) {
		g := NewWithT(t)

		current := semver.MustParse("2.25.0")

		_, err := migrate.DetectPhase(&current, nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("target version"))
	})

	t.Run("should return error when both are nil", func(t *testing.T) {
		g := NewWithT(t)

		_, err := migrate.DetectPhase(nil, nil)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("should handle major version crossing", func(t *testing.T) {
		g := NewWithT(t)

		current := semver.MustParse("2.25.0")
		target := semver.MustParse("3.5.0")

		phase, err := migrate.DetectPhase(&current, &target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(phase).To(Equal(action.PhasePreUpgrade))
	})

	t.Run("should handle minor version difference", func(t *testing.T) {
		g := NewWithT(t)

		current := semver.MustParse("3.0.0")
		target := semver.MustParse("3.5.0")

		phase, err := migrate.DetectPhase(&current, &target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(phase).To(Equal(action.PhasePreUpgrade))
	})
}
