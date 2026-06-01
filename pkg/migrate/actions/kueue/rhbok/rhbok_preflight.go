package rhbok

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/rbac"
)

func preparePermissions() []rbac.PermissionCheck {
	return []rbac.PermissionCheck{
		{Verb: "get", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		{Verb: "list", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		{Verb: "list", Group: resources.ClusterQueue.Group, Resource: resources.ClusterQueue.Resource},
		{Verb: "list", Group: resources.LocalQueue.Group, Resource: resources.LocalQueue.Resource},
		{Verb: "get", Group: resources.ConfigMap.Group, Resource: resources.ConfigMap.Resource, Namespace: applicationsNamespace},
	}
}

func runPermissions() []rbac.PermissionCheck {
	return []rbac.PermissionCheck{
		{Verb: "get", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		{Verb: "list", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		{Verb: "update", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		{Verb: "get", Group: resources.Subscription.Group, Resource: resources.Subscription.Resource, Namespace: operatorNamespace},
		{Verb: "create", Group: resources.Subscription.Group, Resource: resources.Subscription.Resource, Namespace: operatorNamespace},
		{Verb: "list", Group: resources.ClusterServiceVersion.Group, Resource: resources.ClusterServiceVersion.Resource, Namespace: operatorNamespace},
		{Verb: "list", Group: resources.ClusterQueue.Group, Resource: resources.ClusterQueue.Resource},
		{Verb: "list", Group: resources.LocalQueue.Group, Resource: resources.LocalQueue.Resource},
		{Verb: "get", Group: resources.ConfigMap.Group, Resource: resources.ConfigMap.Resource, Namespace: applicationsNamespace},
		{Verb: "update", Group: resources.ConfigMap.Group, Resource: resources.ConfigMap.Resource, Namespace: applicationsNamespace},
	}
}

func (a *RHBOKMigrationAction) verifyRBAC(
	ctx context.Context,
	target action.Target,
	checks []rbac.PermissionCheck,
) {
	step := target.Recorder.Child(
		"verify-rbac",
		"Verify RBAC permissions",
	)

	denied, err := rbac.CheckPermissions(ctx, target.Client.AuthorizationV1(), checks)
	if err != nil {
		step.Complete(result.StepFailed, "Failed to verify RBAC permissions: %v", err)

		return
	}

	if len(denied) > 0 {
		for _, d := range denied {
			step.Child(
				fmt.Sprintf("denied-%s-%s", d.Verb, d.Resource),
				fmt.Sprintf("Missing permission: %s", d),
			).Complete(result.StepFailed, "Permission denied: %s", d)
		}

		step.Complete(result.StepFailed, "%d required permission(s) denied", len(denied))

		return
	}

	step.Complete(result.StepCompleted, "All %d required permissions verified", len(checks))
}

func (a *RHBOKMigrationAction) checkCurrentKueueState(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"check-kueue-state",
		"Verify current Kueue state",
	)

	dsc, err := client.GetSingleton(ctx, target.Client, resources.DataScienceClusterV1)
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Complete(result.StepFailed, "DataScienceCluster not found - OpenShift AI may not be installed")

			return
		}

		step.Complete(result.StepFailed, "Failed to get DataScienceCluster: %v", err)

		return
	}

	managementState, err := jq.Query[string](dsc, ".spec.components.kueue.managementState")
	if err != nil {
		step.Complete(result.StepFailed, "Failed to query Kueue managementState: %v", err)

		return
	}

	if managementState == "" {
		step.Complete(result.StepFailed, "Kueue component not configured in DataScienceCluster")

		return
	}

	step.Complete(result.StepCompleted, "Current Kueue state verified (managementState: %s)", managementState)
}

func (a *RHBOKMigrationAction) checkNoRHBOKConflicts(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"check-rhbok-conflicts",
		"Check for Red Hat build of Kueue operator conflicts",
	)

	subscription, err := target.Client.Dynamic().Resource(resources.Subscription.GVR()).
		Namespace("openshift-kueue-operator").
		Get(ctx, "kueue-operator", metav1.GetOptions{})

	if err == nil && subscription != nil {
		step.Complete(result.StepCompleted, "Red Hat build of Kueue operator already installed - migration may be partially complete")

		return
	}

	if !apierrors.IsNotFound(err) {
		step.Complete(result.StepFailed, "Failed to check Red Hat build of Kueue subscription: %v", err)

		return
	}

	step.Complete(result.StepCompleted, "No Red Hat build of Kueue conflicts detected")
}

func (a *RHBOKMigrationAction) verifyKueueResources(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"verify-kueue-resources",
		"Verify Kueue resources exist",
	)

	clusterQueues, err := target.Client.ListResources(ctx, resources.ClusterQueue.GVR())
	if err != nil {
		step.Complete(result.StepFailed, "Failed to list ClusterQueues: %v", err)

		return
	}

	localQueues, err := target.Client.ListResources(ctx, resources.LocalQueue.GVR())
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Complete(result.StepCompleted,
				"Kueue resources found: %d ClusterQueues (LocalQueue CRD not found)",
				len(clusterQueues))

			return
		}

		step.Complete(result.StepFailed, "Failed to list LocalQueues: %v", err)

		return
	}

	step.Complete(result.StepCompleted,
		"Kueue resources found: %d ClusterQueues, %d LocalQueues",
		len(clusterQueues), len(localQueues))
}
