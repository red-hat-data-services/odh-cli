package deps

import (
	"bytes"
	"context"
	"sync"
	"testing"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func TestShouldInstallDep(t *testing.T) {
	tests := []struct {
		name            string
		targetDeps      []string
		includeOptional bool
		dep             DependencyInfo
		want            bool
	}{
		{
			name:       "target dep matches - always install",
			targetDeps: []string{"cert-manager"},
			dep:        DependencyInfo{Name: "cert-manager", Enabled: "false"},
			want:       true,
		},
		{
			name:       "target dep matches optional - always install",
			targetDeps: []string{"kueue"},
			dep:        DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:       true,
		},
		{
			name:       "required dep - install",
			targetDeps: nil,
			dep:        DependencyInfo{Name: "cert-manager", Enabled: "true"},
			want:       true,
		},
		{
			name:       "optional dep without flag - skip",
			targetDeps: nil,
			dep:        DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:       false,
		},
		{
			name:       "disabled dep without flag - skip",
			targetDeps: nil,
			dep:        DependencyInfo{Name: "servicemesh", Enabled: "false"},
			want:       false,
		},
		{
			name:            "optional dep with flag - install",
			targetDeps:      nil,
			includeOptional: true,
			dep:             DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:            true,
		},
		{
			name:            "disabled dep with flag - still skip",
			targetDeps:      nil,
			includeOptional: true,
			dep:             DependencyInfo{Name: "servicemesh", Enabled: "false"},
			want:            false,
		},
		{
			name:       "different target dep - use normal rules",
			targetDeps: []string{"other-dep"},
			dep:        DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:       false,
		},
		{
			name:       "batch target deps - install matching dep",
			targetDeps: []string{"cert-manager", "kueue"},
			dep:        DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:       true,
		},
		{
			name:       "batch target deps - skip non-matching dep",
			targetDeps: []string{"cert-manager", "kueue"},
			dep:        DependencyInfo{Name: "servicemesh", Enabled: "true"},
			want:       false,
		},
		{
			name:       "explicit empty list - install nothing",
			targetDeps: []string{},
			dep:        DependencyInfo{Name: "cert-manager", Enabled: "true"},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			streams := genericiooptions.IOStreams{
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			}

			cmd := &InstallCommand{
				IO:              iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
				TargetDeps:      tt.targetDeps,
				IncludeOptional: tt.includeOptional,
			}

			got := cmd.shouldInstallDep(tt.dep)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestSlicesEqualUnordered(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{
			name: "equal same order",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "c"},
			want: true,
		},
		{
			name: "equal different order",
			a:    []string{"a", "b", "c"},
			b:    []string{"c", "a", "b"},
			want: true,
		},
		{
			name: "different lengths",
			a:    []string{"a", "b"},
			b:    []string{"a", "b", "c"},
			want: false,
		},
		{
			name: "different elements",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "d"},
			want: false,
		},
		{
			name: "duplicates matter - same counts",
			a:    []string{"a", "a", "b"},
			b:    []string{"a", "b", "a"},
			want: true,
		},
		{
			name: "duplicates matter - different counts",
			a:    []string{"a", "a", "b"},
			b:    []string{"a", "b", "b"},
			want: false,
		},
		{
			name: "both empty",
			a:    []string{},
			b:    []string{},
			want: true,
		},
		{
			name: "one empty",
			a:    []string{"a"},
			b:    []string{},
			want: false,
		},
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := slicesEqualUnordered(tt.a, tt.b)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestRunDryRun(t *testing.T) {
	t.Run("with deps to install", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		cmd := &InstallCommand{
			IO: iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		}

		deps := []DependencyInfo{
			{
				Name:         "cert-manager",
				DisplayName:  "Cert Manager",
				Namespace:    "cert-manager-operator",
				Subscription: "cert-manager",
				Channel:      "stable",
				Enabled:      "true",
			},
		}

		err := cmd.runDryRun(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		g.Expect(output).To(ContainSubstring("[DRY RUN]"))
		g.Expect(output).To(ContainSubstring("Cert Manager"))
		g.Expect(output).To(ContainSubstring("kind: Namespace"))
		g.Expect(output).To(ContainSubstring("kind: OperatorGroup"))
		g.Expect(output).To(ContainSubstring("kind: Subscription"))
		g.Expect(output).To(ContainSubstring("cert-manager-operator"))
		g.Expect(output).To(ContainSubstring("app.kubernetes.io/managed-by: odh-cli"))
	})

	t.Run("bulk mode with no eligible deps shows all-installed", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		// TargetDeps == nil: bulk mode; optional dep without --include-optional is skipped
		cmd := &InstallCommand{
			IO: iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		}

		deps := []DependencyInfo{
			{
				Name:    "kueue",
				Enabled: "auto",
			},
		}

		err := cmd.runDryRun(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		g.Expect(output).To(ContainSubstring("[DRY RUN]"))
		g.Expect(output).To(ContainSubstring(msgAllInstalled))
		g.Expect(output).ToNot(ContainSubstring(msgNoDepsToInstall))
	})

	t.Run("explicit empty TargetDeps shows no-deps-to-install", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		// TargetDeps == []string{}: user explicitly requested nothing
		cmd := &InstallCommand{
			IO:         iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			TargetDeps: []string{},
		}

		deps := []DependencyInfo{
			{
				Name:    "cert-manager",
				Enabled: "true",
			},
		}

		err := cmd.runDryRun(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		g.Expect(output).To(ContainSubstring("[DRY RUN]"))
		g.Expect(output).To(ContainSubstring(msgNoDepsToInstall))
		g.Expect(output).ToNot(ContainSubstring(msgAllInstalled))
	})

	t.Run("with target dep", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		cmd := &InstallCommand{
			IO:         iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			TargetDeps: []string{"kueue"},
		}

		deps := []DependencyInfo{
			{
				Name:        "cert-manager",
				DisplayName: "Cert Manager",
				Namespace:   "cert-manager-operator",
				Enabled:     "true",
			},
			{
				Name:         "kueue",
				DisplayName:  "Kueue",
				Namespace:    "openshift-kueue-operator",
				Subscription: "kueue-operator",
				Channel:      "stable",
				Enabled:      "auto",
			},
		}

		err := cmd.runDryRun(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		// Should only show kueue, not cert-manager
		g.Expect(output).To(ContainSubstring("Kueue"))
		g.Expect(output).ToNot(ContainSubstring("Cert Manager"))
	})
}

func TestRunInstall_EmptyToInstall(t *testing.T) {
	// filterDepsToInstall short-circuits before any API call when shouldInstallDep
	// returns false for all deps, so these tests need no cluster client.

	t.Run("bulk mode with no eligible deps shows all-installed", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		// TargetDeps == nil: bulk mode; all deps disabled, none reach isAlreadyInstalled
		cmd := &InstallCommand{
			IO: iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		}

		deps := []DependencyInfo{
			{Name: "servicemesh", Enabled: "false"},
		}

		err := cmd.runInstall(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(buf.String()).To(ContainSubstring(msgAllInstalled))
		g.Expect(buf.String()).ToNot(ContainSubstring(msgNoDepsToInstall))
	})

	t.Run("explicit empty TargetDeps shows no-deps-to-install", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		// TargetDeps == []string{}: user explicitly requested nothing
		cmd := &InstallCommand{
			IO:         iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			TargetDeps: []string{},
		}

		deps := []DependencyInfo{
			{Name: "cert-manager", Enabled: "true"},
		}

		err := cmd.runInstall(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(buf.String()).To(ContainSubstring(msgNoDepsToInstall))
		g.Expect(buf.String()).ToNot(ContainSubstring(msgAllInstalled))
	})
}

func TestSyncWriter(t *testing.T) {
	t.Run("concurrent writes are safe", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		sw := &syncWriter{w: &buf}

		var wg sync.WaitGroup
		numWriters := 10
		writesPerWriter := 100

		for i := range numWriters {
			wg.Add(1)

			go func(writerID int) {
				defer wg.Done()

				for range writesPerWriter {
					_, err := sw.Write([]byte("x"))
					g.Expect(err).ToNot(HaveOccurred())
				}
			}(i)
		}

		wg.Wait()

		// All writes should complete
		g.Expect(buf.Len()).To(Equal(numWriters * writesPerWriter))
	})

	t.Run("content is preserved", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		sw := &syncWriter{w: &buf}

		_, err := sw.Write([]byte("hello "))
		g.Expect(err).ToNot(HaveOccurred())

		_, err = sw.Write([]byte("world"))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(buf.String()).To(Equal("hello world"))
	})
}

func TestEnsureOperatorGroup(t *testing.T) {
	t.Run("returns false when AlreadyExists", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()
		err := operatorsv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())

		listKinds := map[schema.GroupVersionResource]string{
			operatorsv1.SchemeGroupVersion.WithResource("operatorgroups"): "OperatorGroupList",
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		// Configure fake to return AlreadyExists on Create
		dynamicClient.PrependReactor("create", "operatorgroups", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewAlreadyExists(
				schema.GroupResource{Group: "operators.coreos.com", Resource: "operatorgroups"},
				"test-og",
			)
		})

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := &InstallCommand{
			IO:     iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			client: testClient,
		}

		created, err := cmd.ensureOperatorGroup(ctx, "test-namespace", nil)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(created).To(BeFalse(), "should return false when OG already exists")
	})

	t.Run("returns true when created successfully", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()
		err := operatorsv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())

		listKinds := map[schema.GroupVersionResource]string{
			operatorsv1.SchemeGroupVersion.WithResource("operatorgroups"): "OperatorGroupList",
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := &InstallCommand{
			IO:     iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			client: testClient,
		}

		created, err := cmd.ensureOperatorGroup(ctx, "test-namespace", nil)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(created).To(BeTrue(), "should return true when OG is created")
	})
}
