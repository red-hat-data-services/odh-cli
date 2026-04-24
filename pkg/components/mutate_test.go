package components_test

import (
	"bytes"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/components"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func newMutateTestClient(objects ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func newMutateTestClientWithTracker(objects ...runtime.Object) (client.Client, *dynamicfake.FakeDynamicClient) {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	}), dynamicClient
}

func TestMutateComponentState(t *testing.T) {
	t.Run("mutates component state successfully", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"ray": map[string]any{
				"managementState": "Removed",
			},
		})

		var out bytes.Buffer
		k8sClient := newMutateTestClient(dsc)

		cfg := components.MutateConfig{
			IO:            iostreams.NewIOStreams(nil, &out, &out),
			Client:        k8sClient,
			ComponentName: "ray",
			TargetState:   "Managed",
			ActionVerb:    "enable",
			SkipConfirm:   true,
		}

		err := components.MutateComponentState(ctx, cfg)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("ray"))
		g.Expect(output).To(ContainSubstring("enabled successfully"))
	})

	t.Run("reports no-op when already in target state", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{
				"managementState": "Managed",
			},
		})

		var out bytes.Buffer
		k8sClient := newMutateTestClient(dsc)

		cfg := components.MutateConfig{
			IO:            iostreams.NewIOStreams(nil, &out, &out),
			Client:        k8sClient,
			ComponentName: "dashboard",
			TargetState:   "Managed",
			ActionVerb:    "enable",
			SkipConfirm:   true,
		}

		err := components.MutateComponentState(ctx, cfg)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("already enabled"))
	})

	t.Run("dry-run shows patch without applying", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"trustyai": map[string]any{
				"managementState": "Managed",
			},
		})

		var out bytes.Buffer
		k8sClient, fakeClient := newMutateTestClientWithTracker(dsc)

		cfg := components.MutateConfig{
			IO:            iostreams.NewIOStreams(nil, &out, &out),
			Client:        k8sClient,
			ComponentName: "trustyai",
			TargetState:   "Removed",
			ActionVerb:    "disable",
			DryRun:        true,
		}

		err := components.MutateComponentState(ctx, cfg)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("DRY RUN"))
		g.Expect(output).To(ContainSubstring("trustyai"))
		g.Expect(output).To(ContainSubstring("managementState"))
		g.Expect(output).To(ContainSubstring("Removed"))
		g.Expect(output).To(ContainSubstring("No changes made"))

		// Verify dry-run option was passed to the API
		actions := fakeClient.Actions()
		var patchAction k8stesting.PatchActionImpl
		for _, action := range actions {
			if pa, ok := action.(k8stesting.PatchActionImpl); ok {
				patchAction = pa

				break
			}
		}
		g.Expect(patchAction.Name).ToNot(BeEmpty(), "expected a patch action")
		g.Expect(patchAction.GetPatchType()).To(Equal(types.MergePatchType))
		// Verify DryRun option was set
		patchOpts := patchAction.GetPatchOptions()
		g.Expect(patchOpts.DryRun).To(ContainElement("All"), "dry-run option should be passed to API")
	})

	t.Run("returns error when user aborts confirmation", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"ray": map[string]any{
				"managementState": "Removed",
			},
		})

		var out bytes.Buffer
		in := strings.NewReader("n\n")
		k8sClient := newMutateTestClient(dsc)

		cfg := components.MutateConfig{
			IO:            iostreams.NewIOStreams(in, &out, &out),
			Client:        k8sClient,
			ComponentName: "ray",
			TargetState:   "Managed",
			ActionVerb:    "enable",
			SkipConfirm:   false,
		}

		err := components.MutateComponentState(ctx, cfg)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("aborted"))
	})

	t.Run("proceeds when user confirms", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"ray": map[string]any{
				"managementState": "Removed",
			},
		})

		var out bytes.Buffer
		in := strings.NewReader("y\n")
		k8sClient := newMutateTestClient(dsc)

		cfg := components.MutateConfig{
			IO:            iostreams.NewIOStreams(in, &out, &out),
			Client:        k8sClient,
			ComponentName: "ray",
			TargetState:   "Managed",
			ActionVerb:    "enable",
			SkipConfirm:   false,
		}

		err := components.MutateComponentState(ctx, cfg)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("enabled successfully"))
	})

	t.Run("returns error for non-existent component", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{
				"managementState": "Managed",
			},
		})

		var out bytes.Buffer
		k8sClient := newMutateTestClient(dsc)

		cfg := components.MutateConfig{
			IO:            iostreams.NewIOStreams(nil, &out, &out),
			Client:        k8sClient,
			ComponentName: "nonexistent",
			TargetState:   "Managed",
			ActionVerb:    "enable",
			SkipConfirm:   true,
		}

		err := components.MutateComponentState(ctx, cfg)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("not found"))
	})
}
