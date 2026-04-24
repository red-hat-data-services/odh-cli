//nolint:testpackage // Tests internal implementation
package client

import (
	"context"
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

func createTestDSC() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"dashboard": map[string]any{
						"managementState": "Managed",
					},
				},
			},
		},
	}
}

func TestPatch(t *testing.T) {
	t.Run("applies merge patch successfully", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := createTestDSC()
		scheme := runtime.NewScheme()
		_ = metav1.AddMetaToScheme(scheme)

		listKinds := map[schema.GroupVersionResource]string{
			resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsc)

		client := &defaultClient{
			dynamic: dynamicClient,
		}

		patch := map[string]any{
			"spec": map[string]any{
				"components": map[string]any{
					"ray": map[string]any{
						"managementState": "Managed",
					},
				},
			},
		}

		patchBytes, err := json.Marshal(patch)
		g.Expect(err).ToNot(HaveOccurred())

		result, err := client.Patch(
			ctx,
			resources.DataScienceCluster,
			"default-dsc",
			types.MergePatchType,
			patchBytes,
		)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).ToNot(BeNil())
		g.Expect(result.GetName()).To(Equal("default-dsc"))
	})

	t.Run("returns error for non-existent resource", func(t *testing.T) {
		g := NewWithT(t)
		ctx := context.Background()

		scheme := runtime.NewScheme()
		_ = metav1.AddMetaToScheme(scheme)

		listKinds := map[schema.GroupVersionResource]string{
			resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		client := &defaultClient{
			dynamic: dynamicClient,
		}

		patch := map[string]any{
			"spec": map[string]any{},
		}

		patchBytes, err := json.Marshal(patch)
		g.Expect(err).ToNot(HaveOccurred())

		_, err = client.Patch(
			ctx,
			resources.DataScienceCluster,
			"nonexistent",
			types.MergePatchType,
			patchBytes,
		)

		g.Expect(err).To(HaveOccurred())
	})
}

func TestPatchConfig_Options(t *testing.T) {
	t.Run("WithDryRun sets dry run flag", func(t *testing.T) {
		g := NewWithT(t)

		cfg := &PatchConfig{}
		opt := WithDryRun()
		opt.ApplyTo(cfg)

		g.Expect(cfg.DryRun).To(BeTrue())
	})

	t.Run("WithFieldOwner sets field owner", func(t *testing.T) {
		g := NewWithT(t)

		cfg := &PatchConfig{}
		opt := WithFieldOwner("test-owner")
		opt.ApplyTo(cfg)

		g.Expect(cfg.FieldOwner).To(Equal("test-owner"))
	})

	t.Run("multiple options can be combined", func(t *testing.T) {
		g := NewWithT(t)

		cfg := &PatchConfig{}
		opts := []PatchOption{
			WithDryRun(),
			WithFieldOwner("my-controller"),
		}

		for _, opt := range opts {
			opt.ApplyTo(cfg)
		}

		g.Expect(cfg.DryRun).To(BeTrue())
		g.Expect(cfg.FieldOwner).To(Equal("my-controller"))
	})
}
