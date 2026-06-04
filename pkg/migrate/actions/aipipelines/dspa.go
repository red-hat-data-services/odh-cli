package aipipelines

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

type dspaInfo struct {
	Name      string
	Namespace string
}

func listDSPAs(ctx context.Context, c client.Client, rt resources.ResourceType) ([]dspaInfo, error) {
	return listDSPAsInNamespace(ctx, c, rt, "")
}

func listDSPAsInNamespace(ctx context.Context, c client.Client, rt resources.ResourceType, namespace string) ([]dspaInfo, error) {
	list, err := c.Dynamic().Resource(rt.GVR()).
		Namespace(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", rt.Kind, err)
	}

	dspas := make([]dspaInfo, 0, len(list.Items))

	for _, item := range list.Items {
		dspas = append(dspas, dspaInfo{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
		})
	}

	return dspas, nil
}

type migrateOpts struct {
	DryRun bool
}

func migrateDSPAToV1(
	ctx context.Context,
	c client.Client,
	dspa dspaInfo,
	opts migrateOpts,
) error {
	gvrAlpha := resources.DataSciencePipelinesApplicationV1Alpha1.GVR()
	gvrV1 := resources.DataSciencePipelinesApplicationV1.GVR()

	obj, err := c.Dynamic().Resource(gvrAlpha).
		Namespace(dspa.Namespace).
		Get(ctx, dspa.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting v1alpha1 DSPA %s/%s: %w", dspa.Namespace, dspa.Name, err)
	}

	cleanObj := obj.DeepCopy()
	unstructured.RemoveNestedField(cleanObj.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(cleanObj.Object, "metadata", "generation")
	unstructured.RemoveNestedField(cleanObj.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(cleanObj.Object, "metadata", "uid")
	unstructured.RemoveNestedField(cleanObj.Object, "status")

	cleanObj.SetAPIVersion(resources.DataSciencePipelinesApplicationV1.APIVersion())
	cleanObj.SetKind(resources.DataSciencePipelinesApplicationV1.Kind)

	updateOpts := metav1.UpdateOptions{}
	if opts.DryRun {
		updateOpts.DryRun = []string{metav1.DryRunAll}
	}

	_, err = c.Dynamic().Resource(gvrV1).
		Namespace(dspa.Namespace).
		Update(ctx, cleanObj, updateOpts)
	if err != nil {
		return fmt.Errorf("applying v1 DSPA %s/%s: %w", dspa.Namespace, dspa.Name, err)
	}

	return nil
}

func migrateAllDSPAsToV1(
	ctx context.Context,
	c client.Client,
	recorder action.StepRecorder,
	opts migrateOpts,
) error {
	step := recorder.Child("migrate-v1alpha1-to-v1", "Migrate v1alpha1 DSPAs to v1")

	v1alpha1DSPAs, err := listDSPAs(ctx, c, resources.DataSciencePipelinesApplicationV1Alpha1)
	if err != nil {
		step.Complete(result.StepFailed, "Failed to list v1alpha1 DSPAs: %v", err)

		return err
	}

	if len(v1alpha1DSPAs) == 0 {
		step.Complete(result.StepCompleted, "No v1alpha1 DSPAs found")

		return nil
	}

	step.Record("detected", "Found %d v1alpha1 DSPA(s) to migrate", result.StepCompleted, len(v1alpha1DSPAs))

	if opts.DryRun {
		for _, dspa := range v1alpha1DSPAs {
			step.Record(dspa.Name, "Would migrate %s/%s from v1alpha1 to v1", result.StepSkipped, dspa.Namespace, dspa.Name)
		}

		step.Complete(result.StepSkipped, "Dry-run: %d DSPA(s) would be migrated", len(v1alpha1DSPAs))

		return nil
	}

	backoff := wait.Backoff{
		Duration: retryInitialDuration,
		Factor:   retryFactor,
		Jitter:   retryJitter,
		Steps:    retryMaxSteps,
		Cap:      retryMaxDuration,
	}

	var remaining []dspaInfo

	retryErr := wait.ExponentialBackoff(backoff, func() (bool, error) {
		remaining, err = listDSPAs(ctx, c, resources.DataSciencePipelinesApplicationV1Alpha1)
		if err != nil {
			return false, fmt.Errorf("re-listing v1alpha1 DSPAs: %w", err)
		}

		if len(remaining) == 0 {
			return true, nil
		}

		for _, dspa := range remaining {
			if migrateErr := migrateDSPAToV1(ctx, c, dspa, opts); migrateErr != nil {
				step.Record(dspa.Name, "Migration attempt failed: %v (will retry)", result.StepFailed, migrateErr)
			} else {
				step.Record(dspa.Name, "Migrated %s/%s to v1", result.StepCompleted, dspa.Namespace, dspa.Name)
			}
		}

		remaining, err = listDSPAs(ctx, c, resources.DataSciencePipelinesApplicationV1Alpha1)
		if err != nil {
			return false, fmt.Errorf("verifying migration: %w", err)
		}

		return len(remaining) == 0, nil
	})

	if retryErr != nil {
		step.Complete(result.StepFailed, "Failed to migrate all v1alpha1 DSPAs: %v", retryErr)

		return fmt.Errorf("migrating v1alpha1 DSPAs: %w", retryErr)
	}

	step.Complete(result.StepCompleted, "All v1alpha1 DSPAs migrated to v1")

	return nil
}
