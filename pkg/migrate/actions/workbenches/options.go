package workbenches

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// SharedScopeOptions holds the workbench targeting flags shared by multiple
// workbench actions (e.g., kueue label attach, OAuth cleanup).
type SharedScopeOptions struct {
	WorkbenchNamespace string
	WorkbenchName      string
}

// Validate checks that flag combinations are valid.
func (o *SharedScopeOptions) Validate() error {
	if o.WorkbenchName != "" && o.WorkbenchNamespace == "" {
		return errors.New("--workbench-name requires --workbench-namespace")
	}

	return nil
}

// ListNotebooks returns the notebooks matching the scope filters.
func (o *SharedScopeOptions) ListNotebooks(
	ctx context.Context,
	target action.Target,
) ([]*unstructured.Unstructured, error) {
	if o.WorkbenchName != "" {
		nb, err := target.Client.Dynamic().Resource(resources.Notebook.GVR()).
			Namespace(o.WorkbenchNamespace).
			Get(ctx, o.WorkbenchName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting notebook %s/%s: %w",
				o.WorkbenchNamespace, o.WorkbenchName, err)
		}

		return []*unstructured.Unstructured{nb}, nil
	}

	var opts []client.ListResourcesOption
	if o.WorkbenchNamespace != "" {
		opts = append(opts, client.WithNamespace(o.WorkbenchNamespace))
	}

	nbs, err := target.Client.List(ctx, resources.Notebook, opts...)
	if err != nil {
		return nil, fmt.Errorf("listing notebooks: %w", err)
	}

	return nbs, nil
}

// AddScopeFlags registers the shared workbench targeting flags into fs.
// Guards with fs.Lookup so paired actions that share the same backing
// struct do not trigger flag-collision panics in RegisterActionFlags.
// All actions using these flags must share a single SharedScopeOptions
// instance (see registry.go) so the flag values are visible to every action.
func AddScopeFlags(opts *SharedScopeOptions, fs *pflag.FlagSet) {
	if fs.Lookup("workbench-namespace") == nil {
		fs.StringVar(&opts.WorkbenchNamespace, "workbench-namespace", "",
			"Limit to notebooks in this namespace (default: all namespaces)")
	}

	if fs.Lookup("workbench-name") == nil {
		fs.StringVar(&opts.WorkbenchName, "workbench-name", "",
			"Target a single notebook by name (requires --workbench-namespace)")
	}
}
