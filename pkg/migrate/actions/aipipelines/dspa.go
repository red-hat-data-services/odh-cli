package aipipelines

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
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
	gvrV1 := resources.DataSciencePipelinesApplicationV1.GVR()

	obj, err := c.Dynamic().Resource(gvrV1).
		Namespace(dspa.Namespace).
		Get(ctx, dspa.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting DSPA %s/%s: %w", dspa.Namespace, dspa.Name, err)
	}

	unstructured.RemoveNestedField(obj.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, "metadata", "generation")
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(obj.Object, "metadata", "uid")
	unstructured.RemoveNestedField(obj.Object, "status")

	updateOpts := metav1.UpdateOptions{}
	if opts.DryRun {
		updateOpts.DryRun = []string{metav1.DryRunAll}
	}

	_, err = c.Dynamic().Resource(gvrV1).
		Namespace(dspa.Namespace).
		Update(ctx, obj, updateOpts)
	if err != nil {
		return fmt.Errorf("updating DSPA %s/%s: %w", dspa.Namespace, dspa.Name, err)
	}

	return nil
}

// hasV1Alpha1StoredVersion checks the DSPA CRD's status.storedVersions for v1alpha1.
// Returns (true, nil) if v1alpha1 is present, (false, nil) if not or CRD not found.
// Returns (false, error) only on unexpected errors; permission errors return (true, nil)
// to fall through to the existing migration logic as a safe default.
func hasV1Alpha1StoredVersion(ctx context.Context, c client.Client) (bool, error) {
	crd, err := c.GetResource(ctx, resources.CustomResourceDefinition, dspaCRDName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("getting CRD %s: %w", dspaCRDName, err)
	}

	// nil return means permission error — assume v1alpha1 might exist as a safe default
	if crd == nil {
		return true, nil
	}

	storedVersions, err := jq.Query[[]any](crd, ".status.storedVersions")
	if err != nil {
		return false, fmt.Errorf("querying status.storedVersions for CRD %s: %w", dspaCRDName, err)
	}

	for _, v := range storedVersions {
		if s, ok := v.(string); ok && s == "v1alpha1" {
			return true, nil
		}
	}

	return false, nil
}

// removeV1Alpha1StoredVersion removes v1alpha1 from the DSPA CRD's status.storedVersions.
// Kubernetes does not clean up storedVersions automatically after objects are re-written,
// so this must be done explicitly after a successful migration.
func removeV1Alpha1StoredVersion(ctx context.Context, c client.Client) error {
	crd, err := c.GetResource(ctx, resources.CustomResourceDefinition, dspaCRDName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("getting CRD %s: %w", dspaCRDName, err)
	}

	if crd == nil {
		return nil
	}

	storedVersions, err := jq.Query[[]any](crd, ".status.storedVersions")
	if err != nil {
		return fmt.Errorf("querying status.storedVersions: %w", err)
	}

	filtered := make([]any, 0, len(storedVersions))

	for _, v := range storedVersions {
		if s, ok := v.(string); ok && s == "v1alpha1" {
			continue
		}

		filtered = append(filtered, v)
	}

	if len(filtered) == len(storedVersions) {
		return nil
	}

	if err := unstructured.SetNestedSlice(crd.Object, filtered, "status", "storedVersions"); err != nil {
		return fmt.Errorf("setting status.storedVersions: %w", err)
	}

	_, err = c.Dynamic().Resource(resources.CustomResourceDefinition.GVR()).
		UpdateStatus(ctx, crd, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating CRD status: %w", err)
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

	// Check the CRD's status.storedVersions to determine if v1alpha1 migration is needed.
	// Listing via the v1alpha1 GVR is unreliable: the API server converts stored v1 objects
	// to v1alpha1 on the fly, so that endpoint always returns results regardless of what's
	// actually stored.
	needsMigration, err := hasV1Alpha1StoredVersion(ctx, c)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to check CRD stored versions: %v", err)

		return err
	}

	if !needsMigration {
		step.Completef(result.StepCompleted, "All DSPAs already stored as v1")

		return nil
	}

	dspas, err := listDSPAs(ctx, c, resources.DataSciencePipelinesApplicationV1)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list DSPAs: %v", err)

		return err
	}

	if len(dspas) == 0 {
		if !opts.DryRun {
			if err := removeV1Alpha1StoredVersion(ctx, c); err != nil {
				step.Completef(result.StepFailed, "Failed to clean up storedVersions: %v", err)

				return fmt.Errorf("removing v1alpha1 from storedVersions: %w", err)
			}
		}

		step.Completef(result.StepCompleted, "No DSPAs found")

		return nil
	}

	step.Recordf("detected", "Found %d DSPA(s) to migrate", result.StepCompleted, len(dspas))

	if opts.DryRun {
		for _, dspa := range dspas {
			step.Recordf(dspa.Name, "Would migrate %s/%s from v1alpha1 to v1", result.StepSkipped, dspa.Namespace, dspa.Name)
		}

		step.Completef(result.StepSkipped, "Dry-run: %d DSPA(s) would be migrated", len(dspas))

		return nil
	}

	backoff := wait.Backoff{
		Duration: retryInitialDuration,
		Factor:   retryFactor,
		Jitter:   retryJitter,
		Steps:    retryMaxSteps,
		Cap:      retryMaxDuration,
	}

	migrated := make(map[string]bool, len(dspas))

	retryErr := wait.ExponentialBackoff(backoff, func() (bool, error) {
		allSucceeded := true

		for _, dspa := range dspas {
			key := dspa.Namespace + "/" + dspa.Name
			if migrated[key] {
				continue
			}

			if migrateErr := migrateDSPAToV1(ctx, c, dspa, opts); migrateErr != nil {
				step.Recordf(dspa.Name, "Migration attempt failed: %v (will retry)", result.StepFailed, migrateErr)

				allSucceeded = false
			} else {
				step.Recordf(dspa.Name, "Migrated %s/%s to v1", result.StepCompleted, dspa.Namespace, dspa.Name)

				migrated[key] = true
			}
		}

		return allSucceeded, nil
	})

	if retryErr != nil {
		step.Completef(result.StepFailed, "Failed to migrate all v1alpha1 DSPAs: %v", retryErr)

		return fmt.Errorf("migrating v1alpha1 DSPAs: %w", retryErr)
	}

	if err := removeV1Alpha1StoredVersion(ctx, c); err != nil {
		step.Completef(result.StepFailed, "Failed to remove v1alpha1 from CRD storedVersions: %v", err)

		return fmt.Errorf("removing v1alpha1 from storedVersions: %w", err)
	}

	step.Completef(result.StepCompleted, "All v1alpha1 DSPAs migrated to v1")

	return nil
}
