package action_test

import (
	"testing"

	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"

	. "github.com/onsi/gomega"
)

// mockAction implements Action and ActionConfigurer.
type mockAction struct {
	id    string
	flags map[string]string // Flag name -> default value
}

func (m *mockAction) ID() string                { return m.id }
func (m *mockAction) Name() string              { return "Mock " + m.id }
func (m *mockAction) Description() string       { return "Mock description" }
func (m *mockAction) Group() action.ActionGroup { return action.GroupMigration }

func (m *mockAction) CanApply(target action.Target) bool { return true }
func (m *mockAction) Prepare() action.Task               { return nil }
func (m *mockAction) Run() action.Task                   { return nil }

func (m *mockAction) AddFlags(fs *pflag.FlagSet) {
	for name, val := range m.flags {
		fs.String(name, val, "mock flag")
	}
}

var _ action.ActionConfigurer = (*mockAction)(nil)

func TestRegisterActionFlags(t *testing.T) {
	t.Run("merges distinct flags successfully", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()

		err := registry.Register(&mockAction{
			id:    "action.one",
			flags: map[string]string{"flag-one": "val1"},
		})
		g.Expect(err).NotTo(HaveOccurred())

		err = registry.Register(&mockAction{
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

		err := registry.Register(&mockAction{
			id:    "action.one",
			flags: map[string]string{"shared-flag": "val1"},
		})
		g.Expect(err).NotTo(HaveOccurred())

		err = registry.Register(&mockAction{
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

		err := registry.Register(&mockAction{
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

type mockActionWithoutFlags struct {
	id string
}

func (m *mockActionWithoutFlags) ID() string                         { return m.id }
func (m *mockActionWithoutFlags) Name() string                       { return "Mock " + m.id }
func (m *mockActionWithoutFlags) Description() string                { return "Mock description" }
func (m *mockActionWithoutFlags) Group() action.ActionGroup          { return action.GroupMigration }
func (m *mockActionWithoutFlags) CanApply(target action.Target) bool { return true }
func (m *mockActionWithoutFlags) Prepare() action.Task               { return nil }
func (m *mockActionWithoutFlags) Run() action.Task                   { return nil }
