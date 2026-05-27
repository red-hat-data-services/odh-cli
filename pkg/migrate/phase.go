package migrate

import (
	"errors"
	"fmt"

	"github.com/blang/semver/v4"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

// DetectPhase determines the migration phase based on version comparison.
// If current < target: pre-upgrade (cluster hasn't been upgraded yet).
// If current >= target: post-upgrade (cluster has been upgraded).
func DetectPhase(current, target *semver.Version) (action.ActionPhase, error) {
	if current == nil {
		return "", errors.New("current version is required for phase detection")
	}

	if target == nil {
		return "", errors.New("target version is required for phase detection")
	}

	if current.LT(*target) {
		return action.PhasePreUpgrade, nil
	}

	return action.PhasePostUpgrade, nil
}

type phaseResolverInput struct {
	ParsedPhase    action.ActionPhase
	MigrationIDs   []string
	CurrentVersion *semver.Version
	TargetVersion  *semver.Version
	Registry       *action.ActionRegistry
	Client         client.Client
	IO             iostreams.Interface
}

// resolvePhaseAndMigrations determines the effective phase and populates migration IDs
// when they aren't explicitly provided. Returns the effective phase and the migration IDs.
//
// When --migration is explicit, phase filtering is bypassed: the migration runs
// regardless of phase mismatch, with a warning emitted by the caller.
func resolvePhaseAndMigrations(in phaseResolverInput) (action.ActionPhase, []string, error) {
	effectivePhase := in.ParsedPhase
	if effectivePhase == "" {
		detected, err := DetectPhase(in.CurrentVersion, in.TargetVersion)
		if err != nil {
			return "", nil, fmt.Errorf("detecting phase: %w", err)
		}

		effectivePhase = detected
		in.IO.Errorf("Auto-detected phase: %s", string(effectivePhase))
	}

	migrationIDs := in.MigrationIDs
	if len(migrationIDs) == 0 {
		allActions := in.Registry.ListAll()
		allActions = filterByPhase(allActions, effectivePhase)

		for _, act := range allActions {
			if act.CanApply(action.Target{
				Client:         in.Client,
				CurrentVersion: in.CurrentVersion,
				TargetVersion:  in.TargetVersion,
			}) {
				migrationIDs = append(migrationIDs, act.ID())
			}
		}
	}

	return effectivePhase, migrationIDs, nil
}

func filterByPhase(actions []action.Action, phase action.ActionPhase) []action.Action {
	if phase == "" {
		return actions
	}

	var filtered []action.Action

	for _, a := range actions {
		if a.Phase() == phase {
			filtered = append(filtered, a)
		}
	}

	return filtered
}
