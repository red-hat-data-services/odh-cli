package action

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// RegisterActionFlags registers flags from all ActionConfigurer implementations in the registry
// into the provided FlagSet. It detects naming collisions and panics with an actionable error
// message if an action attempts to register a flag that already exists.
func RegisterActionFlags(registry *ActionRegistry, fs *pflag.FlagSet) {
	const maxParts = 2

	for _, a := range registry.ListAll() {
		configurer, ok := a.(ActionConfigurer)
		if !ok {
			continue
		}

		// Create an isolated FlagSet to collect the action's flags first
		actionFS := pflag.NewFlagSet(a.ID(), pflag.ContinueOnError)
		configurer.AddFlags(actionFS)

		// Merge them into the main FlagSet, checking for collisions
		actionFS.VisitAll(func(f *pflag.Flag) {
			if existing := fs.Lookup(f.Name); existing != nil {
				// Determine a suggested prefix (e.g., "dashboard" from "dashboard.generate-redirect")
				parts := strings.SplitN(a.ID(), ".", maxParts)
				shortPrefix := parts[0]

				panic(fmt.Sprintf(
					"flag --%s registered by action %q conflicts with an existing flag; "+
						"use a unique flag name, e.g., --%s-%s",
					f.Name, a.ID(), shortPrefix, f.Name,
				))
			}

			// Shallow copy the flag allows us to add it safely to the main set
			// Since AddFlag takes a pointer, we must not pass the loop variable directly
			// if we were mutating it, but since we aren't, pflag handles it correctly.
			// However, pflag's AddFlag specifically says "Adds a flag to the FlagSet".
			// We can just add it directly.
			fs.AddFlag(f)
		})
	}
}
