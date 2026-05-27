package migrate

import (
	"bytes"
	"testing"

	"github.com/blang/semver/v4"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

type stubAction struct {
	id          string
	phase       action.ActionPhase
	canApply    bool
	runTask     action.Task
	prepareTask action.Task
}

func (s *stubAction) ID() string                    { return s.id }
func (s *stubAction) Name() string                  { return s.id }
func (s *stubAction) Description() string           { return "" }
func (s *stubAction) Group() action.ActionGroup     { return action.GroupMigration }
func (s *stubAction) Phase() action.ActionPhase     { return s.phase }
func (s *stubAction) CanApply(_ action.Target) bool { return s.canApply }
func (s *stubAction) Prepare() action.Task          { return s.prepareTask }
func (s *stubAction) Run() action.Task              { return s.runTask }

func TestFilterByPhase(t *testing.T) {
	t.Run("should return all actions when phase is empty", func(t *testing.T) {
		g := NewWithT(t)

		actions := []action.Action{
			&stubAction{id: "pre", phase: action.PhasePreUpgrade},
			&stubAction{id: "post", phase: action.PhasePostUpgrade},
		}

		result := filterByPhase(actions, "")
		g.Expect(result).To(HaveLen(2))
	})

	t.Run("should filter to matching phase", func(t *testing.T) {
		g := NewWithT(t)

		actions := []action.Action{
			&stubAction{id: "pre", phase: action.PhasePreUpgrade},
			&stubAction{id: "post", phase: action.PhasePostUpgrade},
			&stubAction{id: "enable", phase: action.PhasePreEnablement},
		}

		result := filterByPhase(actions, action.PhasePreUpgrade)
		g.Expect(result).To(HaveLen(1))
		g.Expect(result[0].ID()).To(Equal("pre"))
	})

	t.Run("should return empty when no actions match", func(t *testing.T) {
		g := NewWithT(t)

		actions := []action.Action{
			&stubAction{id: "pre", phase: action.PhasePreUpgrade},
		}

		result := filterByPhase(actions, action.PhasePostUpgrade)
		g.Expect(result).To(BeEmpty())
	})

	t.Run("should handle nil input", func(t *testing.T) {
		g := NewWithT(t)

		result := filterByPhase(nil, action.PhasePreUpgrade)
		g.Expect(result).To(BeEmpty())
	})
}

func newTestIO() iostreams.Interface {
	return iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
}

func TestResolvePhaseAndMigrations(t *testing.T) {
	t.Run("should use explicit phase when provided", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		io := newTestIO()

		phase, ids, err := resolvePhaseAndMigrations(phaseResolverInput{
			ParsedPhase:  action.PhasePostUpgrade,
			MigrationIDs: []string{"some.id"},
			Registry:     registry,
			IO:           io,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(phase).To(Equal(action.PhasePostUpgrade))
		g.Expect(ids).To(ConsistOf("some.id"))
	})

	t.Run("should auto-detect phase when not provided", func(t *testing.T) {
		g := NewWithT(t)

		current := semver.MustParse("2.25.0")
		target := semver.MustParse("3.0.0")
		registry := action.NewActionRegistry()
		io := newTestIO()

		phase, _, err := resolvePhaseAndMigrations(phaseResolverInput{
			MigrationIDs:   []string{"some.id"},
			CurrentVersion: &current,
			TargetVersion:  &target,
			Registry:       registry,
			IO:             io,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(phase).To(Equal(action.PhasePreUpgrade))
	})

	t.Run("should preserve explicit migration IDs", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&stubAction{id: "other.action", phase: action.PhasePreUpgrade, canApply: true})
		io := newTestIO()

		_, ids, err := resolvePhaseAndMigrations(phaseResolverInput{
			ParsedPhase:  action.PhasePreUpgrade,
			MigrationIDs: []string{"explicit.id"},
			Registry:     registry,
			IO:           io,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ids).To(ConsistOf("explicit.id"))
	})

	t.Run("should resolve IDs from registry when none provided", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&stubAction{id: "pre.action", phase: action.PhasePreUpgrade, canApply: true})
		registry.MustRegister(&stubAction{id: "post.action", phase: action.PhasePostUpgrade, canApply: true})
		io := newTestIO()

		_, ids, err := resolvePhaseAndMigrations(phaseResolverInput{
			ParsedPhase: action.PhasePreUpgrade,
			Registry:    registry,
			IO:          io,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ids).To(ConsistOf("pre.action"))
	})

	t.Run("should filter by CanApply when resolving IDs", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&stubAction{id: "applicable", phase: action.PhasePreUpgrade, canApply: true})
		registry.MustRegister(&stubAction{id: "not-applicable", phase: action.PhasePreUpgrade, canApply: false})
		io := newTestIO()

		_, ids, err := resolvePhaseAndMigrations(phaseResolverInput{
			ParsedPhase: action.PhasePreUpgrade,
			Registry:    registry,
			IO:          io,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ids).To(ConsistOf("applicable"))
	})

	t.Run("should return empty when no actions match phase", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&stubAction{id: "post.only", phase: action.PhasePostUpgrade, canApply: true})
		io := newTestIO()

		_, ids, err := resolvePhaseAndMigrations(phaseResolverInput{
			ParsedPhase: action.PhasePreUpgrade,
			Registry:    registry,
			IO:          io,
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ids).To(BeEmpty())
	})
}
