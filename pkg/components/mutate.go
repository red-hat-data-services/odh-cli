package components

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

// MutateConfig holds common configuration for component state mutations.
type MutateConfig struct {
	IO            iostreams.Interface
	Client        client.Client
	ComponentName string
	TargetState   string
	ActionVerb    string
	DryRun        bool
	SkipConfirm   bool
}

// MutateComponentState changes a component's managementState.
func MutateComponentState(ctx context.Context, cfg MutateConfig) error {
	dsc, err := client.GetDataScienceCluster(ctx, cfg.Client)
	if err != nil {
		return fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	component, err := GetComponentFromDSC(dsc, cfg.ComponentName)
	if err != nil {
		return fmt.Errorf("getting component: %w", err)
	}

	if component.ManagementState == cfg.TargetState {
		cfg.IO.Fprintf("Component '%s' is already %sd (%s)", cfg.ComponentName, cfg.ActionVerb, cfg.TargetState)

		return nil
	}

	patch := buildComponentPatch(cfg.ComponentName, cfg.TargetState)

	patchBytes, err := json.MarshalIndent(patch, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}

	if !cfg.SkipConfirm && !cfg.DryRun {
		prompt := fmt.Sprintf("Are you sure you want to %s component '%s'?", cfg.ActionVerb, cfg.ComponentName)
		if !confirmation.Prompt(cfg.IO, prompt) {
			return ErrUserAborted()
		}
	}

	opts := []client.PatchOption{}
	if cfg.DryRun {
		opts = append(opts, client.WithDryRun())
		cfg.IO.Fprintln("DRY RUN: Validating patch against API server...")
		cfg.IO.Fprintln()
		cfg.IO.Fprintf("%s", string(patchBytes))
		cfg.IO.Fprintln()
	}

	_, err = cfg.Client.Patch(
		ctx,
		resources.DataScienceCluster,
		dsc.GetName(),
		types.MergePatchType,
		patchBytes,
		opts...,
	)
	if err != nil {
		return fmt.Errorf("patching DataScienceCluster: %w", err)
	}

	if cfg.DryRun {
		cfg.IO.Fprintf("DRY RUN: Patch validated successfully. No changes made.")
	} else {
		cfg.IO.Fprintf("Component '%s' %sd successfully (managementState: %s)", cfg.ComponentName, cfg.ActionVerb, cfg.TargetState)
	}

	return nil
}

// buildComponentPatch creates a merge patch for changing component managementState.
func buildComponentPatch(componentName, state string) map[string]any {
	return map[string]any{
		"spec": map[string]any{
			"components": map[string]any{
				componentName: map[string]any{
					"managementState": state,
				},
			},
		},
	}
}
