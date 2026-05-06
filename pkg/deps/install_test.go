package deps_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/deps"

	. "github.com/onsi/gomega"
)

const (
	testDefaultTimeout = 5 * time.Minute
	testFlagDryRun     = "dry-run"
	testFlagOptional   = "include-optional"
	testFlagTimeout    = "timeout"
	testFlagVersion    = "version"
	testFlagRefresh    = "refresh"
)

func TestMatchesSubscription(t *testing.T) {
	tests := []struct {
		name    string
		csvName string
		subName string
		want    bool
	}{
		// Exact prefix matches
		{"exact match", "kueue-operator.v0.10.0", "kueue-operator", true},
		{"exact match cert-manager", "cert-manager.v1.14.0", "cert-manager", true},
		{"exact match with longer version", "kueue-operator.v0.10.0-rc1", "kueue-operator", true},

		// openshift- prefix handling
		{"openshift prefix", "cert-manager-operator.v1.19.0", "openshift-cert-manager-operator", true},
		{"openshift prefix servicemesh", "servicemeshoperator.v2.5.0", "openshift-servicemeshoperator", true},

		// Red Hat -product to -operator naming
		{"product to operator opentelemetry", "opentelemetry-operator.v0.144.0-2", "opentelemetry-product", true},
		{"product to operator tempo", "tempo-operator.v0.20.0-3", "tempo-product", true},

		// Hyphen normalization (equality, not prefix)
		{"hyphen variation jobset", "job-set-operator.v1.0.0", "jobset-operator", true},
		{"hyphen variation reverse", "jobset-operator.v1.0.0", "job-set-operator", true},

		// Auto-append -operator suffix (exact, not normalized)
		{"auto append operator suffix", "cert-manager-operator.v1.0.0", "cert-manager", true},
		{"auto append operator kueue", "kueue-operator.v1.0.0", "kueue", true},

		// Should NOT match - different operators
		{"different operator", "other-operator.v1.0.0", "kueue-operator", false},
		{"partial match should fail", "kueue-operator-extra.v1.0.0", "kueue-operator", false},
		{"prefix false positive prevented", "ab-c-operator.v1.0.0", "abc", false},
		{"substring should fail", "my-kueue-operator.v1.0.0", "kueue-operator", false},
		{"normalized prefix should not match", "abcoperator.v1.0.0", "abc", false},

		// Edge cases
		{"empty csv", "", "kueue-operator", false},
		{"empty subscription", "kueue-operator.v1.0.0", "", false},
		{"no version suffix", "kueue-operator", "kueue-operator", false},
		{"case insensitive", "KUEUE-OPERATOR.v1.0.0", "kueue-operator", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := deps.MatchesSubscription(tt.csvName, tt.subName)
			g.Expect(got).To(Equal(tt.want), "MatchesSubscription(%q, %q)", tt.csvName, tt.subName)
		})
	}
}

func TestInstallCommand(t *testing.T) {
	t.Run("Complete_DryRun", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.DryRun = true

		err := cmd.Complete()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("Defaults", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)

		g.Expect(cmd.DryRun).To(BeFalse())
		g.Expect(cmd.IncludeOptional).To(BeFalse())
		g.Expect(cmd.Timeout).To(Equal(testDefaultTimeout))
		g.Expect(cmd.TargetDep).To(BeEmpty())
	})

	t.Run("AddFlags", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)

		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		cmd.AddFlags(fs)

		g.Expect(fs.Lookup(testFlagDryRun)).ToNot(BeNil())
		g.Expect(fs.Lookup(testFlagOptional)).ToNot(BeNil())
		g.Expect(fs.Lookup(testFlagTimeout)).ToNot(BeNil())
		g.Expect(fs.Lookup(testFlagVersion)).ToNot(BeNil())
		g.Expect(fs.Lookup(testFlagRefresh)).ToNot(BeNil())
	})

	t.Run("Defaults_VersionAndRefresh", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)

		g.Expect(cmd.Version).To(BeEmpty())
		g.Expect(cmd.Refresh).To(BeFalse())
	})

	t.Run("Validate_Success_DryRun", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.DryRun = true
		cmd.Timeout = testDefaultTimeout

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("Validate_InvalidTimeout_Zero", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.DryRun = true
		cmd.Timeout = 0

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("timeout"))
	})

	t.Run("Validate_InvalidTimeout_Negative", func(t *testing.T) {
		g := NewWithT(t)

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := deps.NewInstallCommand(streams, nil)
		cmd.DryRun = true
		cmd.Timeout = -1 * time.Minute

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("timeout"))
	})
}
