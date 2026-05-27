package migrate

import "github.com/opendatahub-io/odh-cli/pkg/migrate/action"

func (c *PrepareCommand) ActionRegistry() *action.ActionRegistry {
	return c.registry
}

func (c *RunCommand) ActionRegistry() *action.ActionRegistry {
	return c.registry
}
