package action_test

import (
	"context"
	"sync"
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"

	. "github.com/onsi/gomega"
)

type mockAction struct {
	id          string
	name        string
	description string
	group       action.ActionGroup
	phase       action.ActionPhase
	canApply    bool
	runTask     action.Task
}

func (m *mockAction) ID() string                    { return m.id }
func (m *mockAction) Name() string                  { return m.name }
func (m *mockAction) Description() string           { return m.description }
func (m *mockAction) Group() action.ActionGroup     { return m.group }
func (m *mockAction) Phase() action.ActionPhase     { return m.phase }
func (m *mockAction) CanApply(_ action.Target) bool { return m.canApply }
func (m *mockAction) Prepare() action.Task          { return nil }
func (m *mockAction) Run() action.Task              { return m.runTask }

type mockTask struct {
	result *result.ActionResult
	err    error
}

func (m *mockTask) Validate(_ context.Context, _ action.Target) (*result.ActionResult, error) {
	return m.result, m.err
}

func (m *mockTask) Execute(_ context.Context, _ action.Target) (*result.ActionResult, error) {
	return m.result, m.err
}

func TestRegistry_Register(t *testing.T) {
	t.Run("should register and retrieve an action", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		act := &mockAction{id: "test.action", name: "Test"}

		err := registry.Register(act)
		g.Expect(err).ToNot(HaveOccurred())

		retrieved, ok := registry.Get("test.action")
		g.Expect(ok).To(BeTrue())
		g.Expect(retrieved.ID()).To(Equal("test.action"))
	})

	t.Run("should return error for duplicate registration", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		act := &mockAction{id: "test.action", name: "Test"}

		err := registry.Register(act)
		g.Expect(err).ToNot(HaveOccurred())

		err = registry.Register(act)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("already registered"))
	})
}

func TestRegistry_MustRegister(t *testing.T) {
	t.Run("should panic on duplicate registration", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		act := &mockAction{id: "test.action", name: "Test"}

		registry.MustRegister(act)

		g.Expect(func() {
			registry.MustRegister(act)
		}).To(Panic())
	})
}

func TestRegistry_Get(t *testing.T) {
	t.Run("should return false for unknown ID", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()

		_, ok := registry.Get("nonexistent")
		g.Expect(ok).To(BeFalse())
	})
}

func TestRegistry_ListAll(t *testing.T) {
	t.Run("should return empty list for empty registry", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		actions := registry.ListAll()
		g.Expect(actions).To(BeEmpty())
	})

	t.Run("should return all actions sorted by ID", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "z.action"})
		registry.MustRegister(&mockAction{id: "a.action"})
		registry.MustRegister(&mockAction{id: "m.action"})

		actions := registry.ListAll()
		g.Expect(actions).To(HaveLen(3))
		g.Expect(actions[0].ID()).To(Equal("a.action"))
		g.Expect(actions[1].ID()).To(Equal("m.action"))
		g.Expect(actions[2].ID()).To(Equal("z.action"))
	})
}

func TestRegistry_ListByPattern(t *testing.T) {
	t.Run("should match by glob pattern", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "kueue.rhbok.migrate", group: action.GroupMigration})
		registry.MustRegister(&mockAction{id: "kueue.other.migrate", group: action.GroupMigration})
		registry.MustRegister(&mockAction{id: "other.action", group: action.GroupBackup})

		matched, err := registry.ListByPattern("kueue.*", "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(matched).To(HaveLen(2))
		g.Expect(matched[0].ID()).To(Equal("kueue.other.migrate"))
		g.Expect(matched[1].ID()).To(Equal("kueue.rhbok.migrate"))
	})

	t.Run("should filter by group", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "a.action", group: action.GroupMigration})
		registry.MustRegister(&mockAction{id: "b.action", group: action.GroupBackup})

		matched, err := registry.ListByPattern("*", action.GroupMigration)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(matched).To(HaveLen(1))
		g.Expect(matched[0].ID()).To(Equal("a.action"))
	})

	t.Run("should return error for invalid pattern", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "test.action"})

		_, err := registry.ListByPattern("[invalid", "")
		g.Expect(err).To(HaveOccurred())
	})
}

func TestRegistry_ListByPhase(t *testing.T) {
	t.Run("should filter by phase", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "pre.action", phase: action.PhasePreUpgrade})
		registry.MustRegister(&mockAction{id: "post.action", phase: action.PhasePostUpgrade})
		registry.MustRegister(&mockAction{id: "enable.action", phase: action.PhasePreEnablement})

		preActions := registry.ListByPhase(action.PhasePreUpgrade)
		g.Expect(preActions).To(HaveLen(1))
		g.Expect(preActions[0].ID()).To(Equal("pre.action"))

		postActions := registry.ListByPhase(action.PhasePostUpgrade)
		g.Expect(postActions).To(HaveLen(1))
		g.Expect(postActions[0].ID()).To(Equal("post.action"))
	})

	t.Run("should return empty for unmatched phase", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "pre.action", phase: action.PhasePreUpgrade})

		postActions := registry.ListByPhase(action.PhasePostUpgrade)
		g.Expect(postActions).To(BeEmpty())
	})

	t.Run("should return sorted by ID", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "z.pre", phase: action.PhasePreUpgrade})
		registry.MustRegister(&mockAction{id: "a.pre", phase: action.PhasePreUpgrade})

		actions := registry.ListByPhase(action.PhasePreUpgrade)
		g.Expect(actions).To(HaveLen(2))
		g.Expect(actions[0].ID()).To(Equal("a.pre"))
		g.Expect(actions[1].ID()).To(Equal("z.pre"))
	})
}

func TestRegistry_ListByFilter(t *testing.T) {
	t.Run("should filter by pattern, group, and phase combined", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "kueue.migrate", group: action.GroupMigration, phase: action.PhasePreUpgrade})
		registry.MustRegister(&mockAction{id: "kueue.backup", group: action.GroupBackup, phase: action.PhasePreUpgrade})
		registry.MustRegister(&mockAction{id: "other.migrate", group: action.GroupMigration, phase: action.PhasePostUpgrade})

		matched, err := registry.ListByFilter("kueue.*", action.GroupMigration, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(matched).To(HaveLen(1))
		g.Expect(matched[0].ID()).To(Equal("kueue.migrate"))
	})

	t.Run("should apply only phase filter when group and pattern are wildcards", func(t *testing.T) {
		g := NewWithT(t)

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "a.action", phase: action.PhasePreUpgrade})
		registry.MustRegister(&mockAction{id: "b.action", phase: action.PhasePostUpgrade})

		matched, err := registry.ListByFilter("*", "", action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(matched).To(HaveLen(1))
		g.Expect(matched[0].ID()).To(Equal("a.action"))
	})
}

func TestRegistry_ConcurrentRegistration(t *testing.T) {
	g := NewWithT(t)

	registry := action.NewActionRegistry()

	var wg sync.WaitGroup

	const numGoroutines = 50

	errors := make([]error, numGoroutines)

	for i := range numGoroutines {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			act := &mockAction{id: "concurrent.action"}
			errors[idx] = registry.Register(act)
		}(i)
	}

	wg.Wait()

	successes := 0

	for _, err := range errors {
		if err == nil {
			successes++
		}
	}

	g.Expect(successes).To(Equal(1))
}
