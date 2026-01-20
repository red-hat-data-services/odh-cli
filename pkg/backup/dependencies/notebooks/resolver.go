package notebooks

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/backup/dependencies"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

// Resolver resolves dependencies for Kubeflow Notebooks.
type Resolver struct{}

//nolint:gochecknoinits // Required for automatic resolver registration
func init() {
	dependencies.MustRegister(&Resolver{})
}

// NewResolver creates a new Notebook dependency resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// CanHandle returns true for Kubeflow Notebook resources.
func (r *Resolver) CanHandle(gvr schema.GroupVersionResource) bool {
	return gvr.Group == resources.Notebook.Group && gvr.Resource == resources.Notebook.Resource
}

// Resolve finds all dependencies for a Notebook.
func (r *Resolver) Resolve(
	ctx context.Context,
	c *client.Client,
	obj *unstructured.Unstructured,
) ([]dependencies.Dependency, error) {
	namespace := obj.GetNamespace()

	volumes, err := r.extractVolumes(obj)
	if err != nil {
		return nil, err
	}

	containers, err := r.extractContainers(obj)
	if err != nil {
		return nil, err
	}

	// Combine volumes and containers for resolution
	sources := make([]any, 0, len(volumes)+len(containers))
	for _, v := range volumes {
		sources = append(sources, v)
	}
	for _, c := range containers {
		sources = append(sources, c)
	}

	var allDeps []dependencies.Dependency

	configMapDeps, err := dependencies.ResolveConfigMaps(ctx, c, namespace, sources...)
	if err != nil {
		return nil, fmt.Errorf("resolving ConfigMaps: %w", err)
	}
	allDeps = append(allDeps, configMapDeps...)

	secretDeps, err := dependencies.ResolveSecrets(ctx, c, namespace, sources...)
	if err != nil {
		return nil, fmt.Errorf("resolving Secrets: %w", err)
	}
	allDeps = append(allDeps, secretDeps...)

	// Pass volumes only for PVCs
	volumeSources := make([]any, 0, len(volumes))
	for _, v := range volumes {
		volumeSources = append(volumeSources, v)
	}

	pvcDeps, err := dependencies.ResolvePVCs(ctx, c, namespace, volumeSources...)
	if err != nil {
		return nil, fmt.Errorf("resolving PVCs: %w", err)
	}
	allDeps = append(allDeps, pvcDeps...)

	return allDeps, nil
}

func (r *Resolver) extractVolumes(obj *unstructured.Unstructured) ([]corev1.Volume, error) {
	volumes, err := jq.Query[[]corev1.Volume](obj, ".spec.volumes // []")
	if err != nil && !errors.Is(err, jq.ErrNotFound) {
		return nil, fmt.Errorf("querying volumes: %w", err)
	}

	return volumes, nil
}

func (r *Resolver) extractContainers(obj *unstructured.Unstructured) ([]corev1.Container, error) {
	containers, err := jq.Query[[]corev1.Container](obj, ".spec.containers // []")
	if err != nil && !errors.Is(err, jq.ErrNotFound) {
		return nil, fmt.Errorf("querying containers: %w", err)
	}

	return containers, nil
}
