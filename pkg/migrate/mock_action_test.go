package migrate_test

import (
	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
)

type MockActionWithFlags struct {
	action.Action
}

func (m *MockActionWithFlags) ID() string                { return "mock.action.flags" }
func (m *MockActionWithFlags) Name() string              { return "Mock Action with Flags" }
func (m *MockActionWithFlags) Description() string       { return "Mock" }
func (m *MockActionWithFlags) Group() action.ActionGroup { return action.GroupMigration }

func (m *MockActionWithFlags) CanApply(target action.Target) bool { return true }
func (m *MockActionWithFlags) Prepare() action.Task               { return nil }
func (m *MockActionWithFlags) Run() action.Task                   { return nil }

func (m *MockActionWithFlags) AddFlags(fs *pflag.FlagSet) {
	fs.String("mock-flag-1", "", "")
	fs.String("mock-flag-2", "", "")
}

var _ action.ActionConfigurer = (*MockActionWithFlags)(nil)
