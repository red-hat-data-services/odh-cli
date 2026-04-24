package components_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/components"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func newDescribeCommandTestClient(objects ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		resources.DataScienceCluster.GVR():                                     resources.DataScienceCluster.ListKind(),
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "dashboards"}: "DashboardList",
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "kserves"}:    "KserveList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func newDescribeCommand(k8sClient client.Client) (*components.DescribeCommand, *bytes.Buffer) {
	var out bytes.Buffer

	streams := genericiooptions.IOStreams{
		In:     nil,
		Out:    &out,
		ErrOut: &out,
	}

	cmd := components.NewDescribeCommand(streams, nil)
	cmd.Client = k8sClient
	cmd.IO = iostreams.NewIOStreams(nil, &out, &out)

	return cmd, &out
}

func TestDescribeCommand_Validate(t *testing.T) {
	t.Run("requires component name", func(t *testing.T) {
		g := NewWithT(t)

		cmd, _ := newDescribeCommand(nil)
		cmd.ComponentName = ""

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("component name is required"))
	})

	t.Run("rejects invalid output format", func(t *testing.T) {
		g := NewWithT(t)

		cmd, _ := newDescribeCommand(nil)
		cmd.ComponentName = "dashboard"
		cmd.OutputFormat = "csv"

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("csv"))
	})
}

func TestDescribeCommand_Run(t *testing.T) {
	t.Run("describes component as table", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{
				"managementState": "Managed",
			},
		})

		dashboardCR := newDashboardCR(true, "")
		k8sClient := newDescribeCommandTestClient(dsc, dashboardCR)
		cmd, out := newDescribeCommand(k8sClient)
		cmd.ComponentName = "dashboard"
		cmd.OutputFormat = "table"

		err := cmd.Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("Name:"))
		g.Expect(output).To(ContainSubstring("dashboard"))
		g.Expect(output).To(ContainSubstring("Management State:"))
		g.Expect(output).To(ContainSubstring("Managed"))
		g.Expect(output).To(ContainSubstring("Ready:"))
		g.Expect(output).To(ContainSubstring("Yes"))
	})

	t.Run("describes component as JSON", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"kserve": map[string]any{
				"managementState": "Removed",
			},
		})

		k8sClient := newDescribeCommandTestClient(dsc)
		cmd, out := newDescribeCommand(k8sClient)
		cmd.ComponentName = "kserve"
		cmd.OutputFormat = "json"

		err := cmd.Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())

		var result map[string]any
		err = json.Unmarshal(out.Bytes(), &result)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result["name"]).To(Equal("kserve"))
		g.Expect(result["managementState"]).To(Equal("Removed"))
	})

	t.Run("returns error for non-existent component", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{
				"managementState": "Managed",
			},
		})

		k8sClient := newDescribeCommandTestClient(dsc)
		cmd, _ := newDescribeCommand(k8sClient)
		cmd.ComponentName = "nonexistent"

		err := cmd.Run(ctx)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("not found"))
	})

	t.Run("includes conditions when available", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{
				"managementState": "Managed",
			},
		})

		dashboardCR := newDashboardCR(true, "All good")
		k8sClient := newDescribeCommandTestClient(dsc, dashboardCR)
		cmd, out := newDescribeCommand(k8sClient)
		cmd.ComponentName = "dashboard"
		cmd.OutputFormat = "table"

		err := cmd.Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("Conditions:"))
		g.Expect(output).To(ContainSubstring("TYPE"))
		g.Expect(output).To(ContainSubstring("STATUS"))
		g.Expect(output).To(ContainSubstring("Ready"))
	})

	t.Run("does not fetch health for Removed components", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"ray": map[string]any{
				"managementState": "Removed",
			},
		})

		k8sClient := newDescribeCommandTestClient(dsc)
		cmd, out := newDescribeCommand(k8sClient)
		cmd.ComponentName = "ray"
		cmd.OutputFormat = "table"

		err := cmd.Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("ray"))
		g.Expect(output).To(ContainSubstring("Removed"))
		g.Expect(output).ToNot(ContainSubstring("Ready:"))
	})
}
