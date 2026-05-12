package logs

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
)

func TestBuildStreamTargets(t *testing.T) {
	t.Run("single pod single container uses container name", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &Command{
			Pods: []*corev1.Pod{
				makePod("pod-1", "container-a"),
			},
		}

		targets := cmd.buildStreamTargets()

		g.Expect(targets).To(HaveLen(1))
		g.Expect(targets[0].Container).To(Equal("container-a"))
		g.Expect(targets[0].Prefix).To(Equal("[container-a] "))
	})

	t.Run("single pod multiple containers expands all with container prefix", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &Command{
			Pods: []*corev1.Pod{
				makePod("pod-1", "app", "sidecar"),
			},
		}

		targets := cmd.buildStreamTargets()

		g.Expect(targets).To(HaveLen(2))
		g.Expect(targets[0].Prefix).To(Equal("[app] "))
		g.Expect(targets[1].Prefix).To(Equal("[sidecar] "))
	})

	t.Run("multiple pods uses pod/container prefix", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &Command{
			Pods: []*corev1.Pod{
				makePod("pod-1", "app"),
				makePod("pod-2", "app"),
			},
		}

		targets := cmd.buildStreamTargets()

		g.Expect(targets).To(HaveLen(2))
		g.Expect(targets[0].Prefix).To(Equal("[pod-1/app] "))
		g.Expect(targets[1].Prefix).To(Equal("[pod-2/app] "))
	})

	t.Run("container flag overrides automatic selection", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &Command{
			Container: "specific",
			Pods: []*corev1.Pod{
				makePod("pod-1", "app", "sidecar"),
			},
		}

		targets := cmd.buildStreamTargets()

		g.Expect(targets).To(HaveLen(1))
		g.Expect(targets[0].Container).To(Equal("specific"))
	})

	t.Run("multiple pods with multiple containers", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &Command{
			Pods: []*corev1.Pod{
				makePod("pod-1", "app", "sidecar"),
				makePod("pod-2", "app"),
			},
		}

		targets := cmd.buildStreamTargets()

		g.Expect(targets).To(HaveLen(3))
		g.Expect(targets[0].Prefix).To(Equal("[pod-1/app] "))
		g.Expect(targets[1].Prefix).To(Equal("[pod-1/sidecar] "))
		g.Expect(targets[2].Prefix).To(Equal("[pod-2/app] "))
	})
}

func makePod(name string, containers ...string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.PodSpec{},
	}

	for _, c := range containers {
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: c})
	}

	return pod
}
