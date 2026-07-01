package kueue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	kueuediscovery "github.com/opendatahub-io/odh-cli/pkg/lint/checks/kueue/discovery"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

const (
	actionID          = "workbenches.attach-kueue-label"
	actionName        = "Attach Kueue queue-name label to workbenches"
	actionDescription = "Adds the kueue.x-k8s.io/queue-name label to notebooks in Kueue-managed namespaces"

	defaultQueueName = "default"

	minTargetMajorVersion = 3
)

// AttachKueueLabelAction implements the workbenches.attach-kueue-label migration action.
// It adds the kueue.x-k8s.io/queue-name label to notebooks in namespaces
// that have the kueue.openshift.io/managed=true label.
type AttachKueueLabelAction struct {
	QueueName          string
	WorkbenchNamespace string
	WorkbenchName      string
}

func (a *AttachKueueLabelAction) ID() string          { return actionID }
func (a *AttachKueueLabelAction) Name() string        { return actionName }
func (a *AttachKueueLabelAction) Description() string { return actionDescription }

func (a *AttachKueueLabelAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *AttachKueueLabelAction) Phase() action.ActionPhase {
	return action.PhasePostUpgrade
}

func (a *AttachKueueLabelAction) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&a.QueueName, "queue-name", defaultQueueName,
		"Queue name value for the kueue.x-k8s.io/queue-name label")
	fs.StringVar(&a.WorkbenchNamespace, "workbench-namespace", "",
		"Limit to notebooks in this namespace (default: all namespaces)")
	fs.StringVar(&a.WorkbenchName, "workbench-name", "",
		"Target a single notebook by name (requires --workbench-namespace)")
}

func (a *AttachKueueLabelAction) CanApply(target action.Target) bool {
	if target.TargetVersion == nil {
		return false
	}

	return target.TargetVersion.Major >= minTargetMajorVersion
}

func (a *AttachKueueLabelAction) Prepare() action.Task {
	return nil
}

func (a *AttachKueueLabelAction) Run() action.Task {
	return &runTask{action: a}
}

func (a *AttachKueueLabelAction) promptBeforeModification(
	target action.Target,
	count int,
) bool {
	if target.SkipConfirm {
		return true
	}

	target.IO.Fprintln()
	target.IO.Errorf("About to add the Kueue queue-name label to %d Notebook(s)", count)

	return confirmation.Prompt(target.IO, "Proceed with label modifications?")
}

func (a *AttachKueueLabelAction) queueName() string {
	if a.QueueName == "" {
		return defaultQueueName
	}

	return a.QueueName
}

func (a *AttachKueueLabelAction) listNotebooks(
	ctx context.Context,
	target action.Target,
) ([]*unstructured.Unstructured, error) {
	if a.WorkbenchName != "" {
		nb, err := target.Client.Dynamic().Resource(resources.Notebook.GVR()).
			Namespace(a.WorkbenchNamespace).
			Get(ctx, a.WorkbenchName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting notebook %s/%s: %w", a.WorkbenchNamespace, a.WorkbenchName, err)
		}

		return []*unstructured.Unstructured{nb}, nil
	}

	var opts []client.ListResourcesOption
	if a.WorkbenchNamespace != "" {
		opts = append(opts, client.WithNamespace(a.WorkbenchNamespace))
	}

	nbs, err := target.Client.List(ctx, resources.Notebook, opts...)
	if err != nil {
		return nil, fmt.Errorf("listing notebooks: %w", err)
	}

	return nbs, nil
}

func (a *AttachKueueLabelAction) kueueManagedNamespaces(
	ctx context.Context,
	target action.Target,
) (sets.Set[string], error) {
	ns, err := kueuediscovery.KueueEnabledNamespaces(ctx, target.Client)
	if err != nil {
		return nil, fmt.Errorf("listing Kueue-enabled namespaces: %w", err)
	}

	return ns, nil
}

func hasQueueNameLabel(nb *unstructured.Unstructured) bool {
	labels := nb.GetLabels()
	if labels == nil {
		return false
	}

	_, exists := labels[constants.LabelKueueQueueName]

	return exists
}

func (a *AttachKueueLabelAction) patchLabel(
	ctx context.Context,
	target action.Target,
	nb *unstructured.Unstructured,
) error {
	patch := map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{
				constants.LabelKueueQueueName: a.queueName(),
			},
		},
	}

	patchData, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}

	_, err = target.Client.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace(nb.GetNamespace()).
		Patch(ctx, nb.GetName(), types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("patching notebook %s/%s: %w", nb.GetNamespace(), nb.GetName(), err)
	}

	return nil
}
