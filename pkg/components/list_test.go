package components_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"

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

func newListCommandTestClient(objects ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		resources.DataScienceCluster.GVR():                                               resources.DataScienceCluster.ListKind(),
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "dashboards"}:           "DashboardList",
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "kserves"}:              "KserveList",
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "rays"}:                 "RayList",
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "workbenches"}:          "WorkbenchesList",
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "trustyais"}:            "TrustyAIList",
		{Group: componentCRGroup, Version: "v1alpha1", Resource: "datasciencepipelines"}: "DataSciencePipelinesList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func newListCommand(k8sClient client.Client) (*components.ListCommand, *bytes.Buffer) {
	var out bytes.Buffer

	streams := genericiooptions.IOStreams{
		In:     nil,
		Out:    &out,
		ErrOut: &out,
	}

	cmd := components.NewListCommand(streams, nil)
	cmd.Client = k8sClient
	cmd.IO = iostreams.NewIOStreams(nil, &out, &out)

	return cmd, &out
}

func TestListCommand_Validate(t *testing.T) {
	t.Run("rejects invalid output format", func(t *testing.T) {
		g := NewWithT(t)

		cmd, _ := newListCommand(nil)
		cmd.OutputFormat = "xml"

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("xml"))
	})
}

func TestListCommand_Run(t *testing.T) {
	t.Run("lists components as table", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{
				"managementState": "Managed",
			},
			"kserve": map[string]any{
				"managementState": "Removed",
			},
		})

		k8sClient := newListCommandTestClient(dsc)
		cmd, out := newListCommand(k8sClient)
		cmd.OutputFormat = "table"

		err := cmd.Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("NAME"))
		g.Expect(output).To(ContainSubstring("STATE"))
		g.Expect(output).To(ContainSubstring("dashboard"))
		g.Expect(output).To(ContainSubstring("Managed"))
		g.Expect(output).To(ContainSubstring("kserve"))
		g.Expect(output).To(ContainSubstring("Removed"))
	})

	t.Run("lists components as JSON", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"ray": map[string]any{
				"managementState": "Managed",
			},
		})

		k8sClient := newListCommandTestClient(dsc)
		cmd, out := newListCommand(k8sClient)
		cmd.OutputFormat = "json"

		err := cmd.Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring(`"components"`))
		g.Expect(output).To(ContainSubstring(`"name"`))
		g.Expect(output).To(ContainSubstring(`"ray"`))
	})

	t.Run("lists components as YAML", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"workbenches": map[string]any{
				"managementState": "Unmanaged",
			},
		})

		k8sClient := newListCommandTestClient(dsc)
		cmd, out := newListCommand(k8sClient)
		cmd.OutputFormat = "yaml"

		err := cmd.Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("components:"))
		g.Expect(output).To(ContainSubstring("workbenches"))
		g.Expect(output).To(ContainSubstring("Unmanaged"))
	})
}

func TestListCommand_RunWithHealth(t *testing.T) {
	t.Run("enriches managed components with health info", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := newDSC(map[string]any{
			"dashboard": map[string]any{
				"managementState": "Managed",
			},
		})

		dashboardCR := newDashboardCR(true, "")
		k8sClient := newListCommandTestClient(dsc, dashboardCR)
		cmd, out := newListCommand(k8sClient)

		err := cmd.Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())

		output := out.String()
		g.Expect(output).To(ContainSubstring("dashboard"))
		g.Expect(output).To(ContainSubstring("Yes"))
	})
}

// --- Output function tests ---

func TestOutputTable(t *testing.T) {
	t.Run("shows MESSAGE column when components have messages", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		ready := true
		notReady := false

		comps := []components.ComponentInfo{
			{Name: "dashboard", ManagementState: "Managed", Ready: &ready},
			{Name: "ray", ManagementState: "Managed", Ready: &notReady, Message: "Degraded"},
		}

		err := components.OutputTable(&buf, comps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		g.Expect(output).To(ContainSubstring("NAME"))
		g.Expect(output).To(ContainSubstring("STATE"))
		g.Expect(output).To(ContainSubstring("READY"))
		g.Expect(output).To(ContainSubstring("MESSAGE"))
		g.Expect(output).To(ContainSubstring("Degraded"))
	})

	t.Run("hides MESSAGE column when no components have messages", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		ready := true

		comps := []components.ComponentInfo{
			{Name: "dashboard", ManagementState: "Managed", Ready: &ready},
			{Name: "kserve", ManagementState: "Removed", Ready: nil},
		}

		err := components.OutputTable(&buf, comps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		g.Expect(output).To(ContainSubstring("NAME"))
		g.Expect(output).To(ContainSubstring("STATE"))
		g.Expect(output).To(ContainSubstring("READY"))
		g.Expect(output).ToNot(ContainSubstring("MESSAGE"))
	})
}

func TestOutputJSON(t *testing.T) {
	t.Run("renders components as valid JSON", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		ready := true

		comps := []components.ComponentInfo{
			{Name: "dashboard", ManagementState: "Managed", Ready: &ready},
			{Name: "kserve", ManagementState: "Removed"},
		}

		err := components.OutputJSON(&buf, comps)

		g.Expect(err).ToNot(HaveOccurred())

		var result map[string]any
		err = json.Unmarshal(buf.Bytes(), &result)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(result).To(HaveKey("components"))

		compList, ok := result["components"].([]any)
		g.Expect(ok).To(BeTrue())
		g.Expect(compList).To(HaveLen(2))

		first := compList[0].(map[string]any)
		g.Expect(first["name"]).To(Equal("dashboard"))
		g.Expect(first["managementState"]).To(Equal("Managed"))
		g.Expect(first["ready"]).To(BeTrue())
	})

	t.Run("omits nil ready field", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer

		comps := []components.ComponentInfo{
			{Name: "kserve", ManagementState: "Removed"},
		}

		err := components.OutputJSON(&buf, comps)

		g.Expect(err).ToNot(HaveOccurred())

		var result map[string]any
		err = json.Unmarshal(buf.Bytes(), &result)
		g.Expect(err).ToNot(HaveOccurred())

		compList := result["components"].([]any)
		first := compList[0].(map[string]any)
		g.Expect(first).ToNot(HaveKey("ready"))
	})
}

func TestOutputYAML(t *testing.T) {
	t.Run("renders components as valid YAML", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		ready := false

		comps := []components.ComponentInfo{
			{Name: "ray", ManagementState: "Unmanaged", Ready: &ready, Message: "Not ready"},
		}

		err := components.OutputYAML(&buf, comps)

		g.Expect(err).ToNot(HaveOccurred())

		var result map[string]any
		err = yaml.Unmarshal(buf.Bytes(), &result)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(result).To(HaveKey("components"))

		compList := result["components"].([]any)
		g.Expect(compList).To(HaveLen(1))

		first := compList[0].(map[string]any)
		g.Expect(first["name"]).To(Equal("ray"))
		g.Expect(first["managementState"]).To(Equal("Unmanaged"))
		g.Expect(first["ready"]).To(BeFalse())
		g.Expect(first["message"]).To(Equal("Not ready"))
	})
}
