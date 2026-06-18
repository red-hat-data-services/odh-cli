package action_test

import (
	"testing"

	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"

	. "github.com/onsi/gomega"
)

// mockActionWithFlags implements Action and ActionConfigurer.
type mockActionWithFlags struct {
	id    string
	flags map[string]string // Flag name -> default value
}

func (m *mockActionWithFlags) ID() string                { return m.id }
func (m *mockActionWithFlags) Name() string              { return "Mock " + m.id }
func (m *mockActionWithFlags) Description() string       { return "Mock description" }
func (m *mockActionWithFlags) Group() action.ActionGroup { return action.GroupMigration }
func (m *mockActionWithFlags) Phase() action.ActionPhase { return action.PhasePreUpgrade }

func (m *mockActionWithFlags) CanApply(target action.Target) bool { return true }
func (m *mockActionWithFlags) Prepare() action.Task               { return nil }
func (m *mockActionWithFlags) Run() action.Task                   { return nil }

func (m *mockActionWithFlags) AddFlags(fs *pflag.FlagSet) {
	for name, val := range m.flags {
		fs.String(name, val, "mock flag")
	}
}

var _ action.ActionConfigurer = (*mockActionWithFlags)(nil)

func TestRegisterActionFlags(t *testing.T) {
	t.Run("merges distinct flags successfully", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()

		err := registry.Register(&mockActionWithFlags{
			id:    "action.one",
			flags: map[string]string{"flag-one": "val1"},
		})
		g.Expect(err).NotTo(HaveOccurred())

		err = registry.Register(&mockActionWithFlags{
			id:    "action.two",
			flags: map[string]string{"flag-two": "val2"},
		})
		g.Expect(err).NotTo(HaveOccurred())

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

		action.RegisterActionFlags(registry, fs)

		g.Expect(fs.Lookup("flag-one")).NotTo(BeNil())
		g.Expect(fs.Lookup("flag-two")).NotTo(BeNil())
	})

	t.Run("panics on collision with another action", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()

		err := registry.Register(&mockActionWithFlags{
			id:    "action.one",
			flags: map[string]string{"shared-flag": "val1"},
		})
		g.Expect(err).NotTo(HaveOccurred())

		err = registry.Register(&mockActionWithFlags{
			id:    "action.two",
			flags: map[string]string{"shared-flag": "val2"},
		})
		g.Expect(err).NotTo(HaveOccurred())

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

		g.Expect(func() {
			action.RegisterActionFlags(registry, fs)
		}).To(PanicWith(MatchRegexp(`flag --shared-flag registered by action "action.(one|two)" conflicts with an existing flag; use a unique flag name, e.g., --action-shared-flag`)))
	})

	t.Run("panics on collision with command flag", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()

		err := registry.Register(&mockActionWithFlags{
			id:    "dashboard.redirect",
			flags: map[string]string{"dry-run": "false"},
		})
		g.Expect(err).NotTo(HaveOccurred())

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.Bool("dry-run", false, "command flag")

		g.Expect(func() {
			action.RegisterActionFlags(registry, fs)
		}).To(PanicWith(MatchRegexp(`flag --dry-run registered by action "dashboard.redirect" conflicts with an existing flag; use a unique flag name, e.g., --dashboard-dry-run`)))
	})

	t.Run("allows shared flags bound to same variable", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()

		var shared string
		err := registry.Register(&mockSharedFlagAction{
			id: "rc.backup", target: &shared, flagName: "rc-cluster",
		})
		g.Expect(err).NotTo(HaveOccurred())

		err = registry.Register(&mockSharedFlagAction{
			id: "rc.migrate", target: &shared, flagName: "rc-cluster",
		})
		g.Expect(err).NotTo(HaveOccurred())

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

		g.Expect(func() {
			action.RegisterActionFlags(registry, fs)
		}).NotTo(Panic())

		g.Expect(fs.Lookup("rc-cluster")).NotTo(BeNil())

		err = fs.Parse([]string{"--rc-cluster", "my-cluster"})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(shared).To(Equal("my-cluster"))
	})

	t.Run("panics on same flag name from unrelated actions", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()

		err := registry.Register(&mockActionWithFlags{
			id:    "action.one",
			flags: map[string]string{"shared-name": ""},
		})
		g.Expect(err).NotTo(HaveOccurred())

		err = registry.Register(&mockActionWithFlags{
			id:    "action.two",
			flags: map[string]string{"shared-name": ""},
		})
		g.Expect(err).NotTo(HaveOccurred())

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

		g.Expect(func() {
			action.RegisterActionFlags(registry, fs)
		}).To(PanicWith(MatchRegexp(`flag --shared-name registered by action "action\.(one|two)" conflicts`)))
	})

	t.Run("ignores actions without ActionConfigurer", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()

		// action that doesn't implement ActionConfigurer
		err := registry.Register(&mockActionWithoutFlags{id: "no.flags"})
		g.Expect(err).NotTo(HaveOccurred())

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

		g.Expect(func() {
			action.RegisterActionFlags(registry, fs)
		}).NotTo(Panic())
	})
}

type mockSharedFlagAction struct {
	id       string
	target   *string
	flagName string
}

func (m *mockSharedFlagAction) ID() string                         { return m.id }
func (m *mockSharedFlagAction) Name() string                       { return "Mock " + m.id }
func (m *mockSharedFlagAction) Description() string                { return "Mock description" }
func (m *mockSharedFlagAction) Group() action.ActionGroup          { return action.GroupMigration }
func (m *mockSharedFlagAction) Phase() action.ActionPhase          { return action.PhasePreUpgrade }
func (m *mockSharedFlagAction) CanApply(target action.Target) bool { return true }
func (m *mockSharedFlagAction) Prepare() action.Task               { return nil }
func (m *mockSharedFlagAction) Run() action.Task                   { return nil }

func (m *mockSharedFlagAction) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(m.target, m.flagName, "", "shared flag")
}

var _ action.ActionConfigurer = (*mockSharedFlagAction)(nil)

type mockActionWithoutFlags struct {
	id string
}

func (m *mockActionWithoutFlags) ID() string                         { return m.id }
func (m *mockActionWithoutFlags) Name() string                       { return "Mock " + m.id }
func (m *mockActionWithoutFlags) Description() string                { return "Mock description" }
func (m *mockActionWithoutFlags) Group() action.ActionGroup          { return action.GroupMigration }
func (m *mockActionWithoutFlags) Phase() action.ActionPhase          { return action.PhasePreUpgrade }
func (m *mockActionWithoutFlags) CanApply(target action.Target) bool { return true }
func (m *mockActionWithoutFlags) Prepare() action.Task               { return nil }
func (m *mockActionWithoutFlags) Run() action.Task                   { return nil }
