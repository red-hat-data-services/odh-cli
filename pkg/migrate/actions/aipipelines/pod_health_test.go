package aipipelines_test

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/aipipelines"

	. "github.com/onsi/gomega"
)

func TestGetPodGroup(t *testing.T) {
	t.Run("healthy pods matching prefix", func(t *testing.T) {
		g := NewWithT(t)

		pods := []unstructured.Unstructured{
			aipipelines.MakePodUnstructured("ds-pipeline-mydspa-abc123", "ns1", "Running", "True"),
			aipipelines.MakePodUnstructured("ds-pipeline-mydspa-def456", "ns1", "Running", "True"),
			aipipelines.MakePodUnstructured("mariadb-mydspa-0", "ns1", "Running", "True"),
		}

		group := aipipelines.GetPodGroup(pods, "ds-pipeline-mydspa")

		g.Expect(group.Prefix).To(Equal("ds-pipeline-mydspa"))
		g.Expect(group.PodsFound).To(BeTrue())
		g.Expect(group.AllHealthy).To(BeTrue())
		g.Expect(group.Pods).To(HaveLen(2))
		g.Expect(group.Pods[0].Name).To(Equal("ds-pipeline-mydspa-abc123"))
		g.Expect(group.Pods[0].Healthy).To(BeTrue())
	})

	t.Run("unhealthy pod in group", func(t *testing.T) {
		g := NewWithT(t)

		pods := []unstructured.Unstructured{
			aipipelines.MakePodUnstructured("ds-pipeline-mydspa-abc", "ns1", "Running", "True"),
			aipipelines.MakePodUnstructured("ds-pipeline-mydspa-def", "ns1", "Pending", "False"),
		}

		group := aipipelines.GetPodGroup(pods, "ds-pipeline-mydspa")

		g.Expect(group.PodsFound).To(BeTrue())
		g.Expect(group.AllHealthy).To(BeFalse())
		g.Expect(group.Pods[1].Healthy).To(BeFalse())
		g.Expect(group.Pods[1].Phase).To(Equal("Pending"))
	})

	t.Run("no pods matching prefix", func(t *testing.T) {
		g := NewWithT(t)

		pods := []unstructured.Unstructured{
			aipipelines.MakePodUnstructured("other-pod-abc", "ns1", "Running", "True"),
		}

		group := aipipelines.GetPodGroup(pods, "ds-pipeline-mydspa")

		g.Expect(group.PodsFound).To(BeFalse())
		g.Expect(group.AllHealthy).To(BeFalse())
		g.Expect(group.Pods).To(BeEmpty())
	})

	t.Run("pod with missing status", func(t *testing.T) {
		g := NewWithT(t)

		pods := []unstructured.Unstructured{
			aipipelines.MakePodUnstructured("ds-pipeline-mydspa-abc", "ns1", "", ""),
		}

		group := aipipelines.GetPodGroup(pods, "ds-pipeline-mydspa")

		g.Expect(group.PodsFound).To(BeTrue())
		g.Expect(group.AllHealthy).To(BeFalse())
		g.Expect(group.Pods[0].Phase).To(Equal("Unknown"))
		g.Expect(group.Pods[0].Ready).To(Equal("Unknown"))
		g.Expect(group.Pods[0].Healthy).To(BeFalse())
	})

	t.Run("empty pod list", func(t *testing.T) {
		g := NewWithT(t)

		group := aipipelines.GetPodGroup(nil, "ds-pipeline-mydspa")

		g.Expect(group.PodsFound).To(BeFalse())
		g.Expect(group.AllHealthy).To(BeFalse())
	})
}

func TestSaveAndLoadPodHealthState(t *testing.T) {
	t.Run("round-trip", func(t *testing.T) {
		g := NewWithT(t)

		state := aipipelines.PodHealthState{
			CapturedAt: "2026-05-27T10:00:00Z",
			DSPAs: []aipipelines.DSPAState{
				{
					Name:      "mydspa",
					Namespace: "ns1",
					PodGroups: []aipipelines.PodGroup{
						{
							Prefix:     "ds-pipeline-mydspa",
							PodsFound:  true,
							AllHealthy: true,
							Pods: []aipipelines.PodInfo{
								{Name: "ds-pipeline-mydspa-abc", Phase: "Running", Ready: "True", Healthy: true},
							},
						},
					},
				},
			},
		}

		dir := t.TempDir()
		path := filepath.Join(dir, "state.json")

		err := aipipelines.SavePodHealthState(state, path)
		g.Expect(err).ToNot(HaveOccurred())

		loaded, err := aipipelines.LoadPodHealthState(path)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(loaded.CapturedAt).To(Equal("2026-05-27T10:00:00Z"))
		g.Expect(loaded.DSPAs).To(HaveLen(1))
		g.Expect(loaded.DSPAs[0].Name).To(Equal("mydspa"))
		g.Expect(loaded.DSPAs[0].PodGroups[0].AllHealthy).To(BeTrue())
	})

	t.Run("save to multiple paths", func(t *testing.T) {
		g := NewWithT(t)

		state := aipipelines.PodHealthState{CapturedAt: "2026-01-01T00:00:00Z"}

		dir := t.TempDir()
		path1 := filepath.Join(dir, "a", "state.json")
		path2 := filepath.Join(dir, "b", "state.json")

		err := aipipelines.SavePodHealthState(state, path1, path2)
		g.Expect(err).ToNot(HaveOccurred())

		_, err = os.Stat(path1)
		g.Expect(err).ToNot(HaveOccurred())

		_, err = os.Stat(path2)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("load from missing file", func(t *testing.T) {
		g := NewWithT(t)

		_, err := aipipelines.LoadPodHealthState("/nonexistent/path/state.json")
		g.Expect(err).To(HaveOccurred())
	})
}

func TestHasAnyDegradation(t *testing.T) {
	t.Skip("cannot construct podGroupComparison from outside the package; tested via postcheck integration tests")
}

func TestDefaultStatePath(t *testing.T) {
	t.Run("returns path under /tmp provisioned directory", func(t *testing.T) {
		g := NewWithT(t)

		path := aipipelines.DefaultStatePath()
		g.Expect(path).To(ContainSubstring("/tmp/rhoai-upgrade-backup"))
		g.Expect(path).To(ContainSubstring("ai_pipelines"))
		g.Expect(path).To(HaveSuffix("dspa_pre_upgrade_pods.json"))
	})
}
