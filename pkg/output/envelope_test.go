package output_test

import (
	"testing"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/output"

	. "github.com/onsi/gomega"
)

func TestNewMetadata_PopulatesGeneratedAt(t *testing.T) {
	g := NewWithT(t)

	before := time.Now().UTC().Truncate(time.Second)
	meta := output.NewMetadata("lint")
	after := time.Now().UTC().Truncate(time.Second).Add(time.Second)

	g.Expect(meta.GeneratedAt).ToNot(BeEmpty())

	parsed, err := time.Parse(time.RFC3339, meta.GeneratedAt)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(parsed).To(BeTemporally(">=", before))
	g.Expect(parsed).To(BeTemporally("<=", after))
}

func TestNewMetadata_SetsCommand(t *testing.T) {
	g := NewWithT(t)

	t.Run("lint command", func(t *testing.T) {
		meta := output.NewMetadata("lint")
		g.Expect(meta.Command).To(Equal("lint"))
	})

	t.Run("backup command", func(t *testing.T) {
		meta := output.NewMetadata("backup")
		g.Expect(meta.Command).To(Equal("backup"))
	})

	t.Run("status command", func(t *testing.T) {
		meta := output.NewMetadata("status")
		g.Expect(meta.Command).To(Equal("status"))
	})
}

func TestNewMetadata_SetsCLIVersion(t *testing.T) {
	g := NewWithT(t)

	meta := output.NewMetadata("lint")
	g.Expect(meta.CLIVersion).ToNot(BeEmpty())
}

func TestAPIVersion_IsStable(t *testing.T) {
	g := NewWithT(t)

	g.Expect(output.APIVersion).To(Equal("cli.opendatahub.io/v1"))
}

func TestNewStatus_Success(t *testing.T) {
	g := NewWithT(t)

	status := output.NewStatus(0, 0)

	g.Expect(status.Result).To(Equal(output.StatusSuccess))
	g.Expect(status.Warnings).To(Equal(0))
	g.Expect(status.Errors).To(Equal(0))
}

func TestNewStatus_Warning(t *testing.T) {
	g := NewWithT(t)

	status := output.NewStatus(3, 0)

	g.Expect(status.Result).To(Equal(output.StatusWarning))
	g.Expect(status.Warnings).To(Equal(3))
	g.Expect(status.Errors).To(Equal(0))
}

func TestNewStatus_Failure(t *testing.T) {
	g := NewWithT(t)

	status := output.NewStatus(2, 1)

	g.Expect(status.Result).To(Equal(output.StatusFailure))
	g.Expect(status.Warnings).To(Equal(2))
	g.Expect(status.Errors).To(Equal(1))
}

func TestNewStatus_FailureTakesPrecedence(t *testing.T) {
	g := NewWithT(t)

	// Even with warnings, errors should result in failure
	status := output.NewStatus(5, 3)

	g.Expect(status.Result).To(Equal(output.StatusFailure))
}

func TestStatusConstants(t *testing.T) {
	g := NewWithT(t)

	g.Expect(output.StatusSuccess).To(Equal("success"))
	g.Expect(output.StatusWarning).To(Equal("warning"))
	g.Expect(output.StatusFailure).To(Equal("failure"))
}

func TestNewEnvelope(t *testing.T) {
	g := NewWithT(t)

	env := output.NewEnvelope("TestKind", "test-cmd")

	g.Expect(env.APIVersion).To(Equal(output.APIVersion))
	g.Expect(env.Kind).To(Equal("TestKind"))
	g.Expect(env.Metadata.Command).To(Equal("test-cmd"))
	g.Expect(env.Metadata.GeneratedAt).ToNot(BeEmpty())
	g.Expect(env.Status).To(BeNil())
}

func TestEnvelope_SetStatus(t *testing.T) {
	g := NewWithT(t)

	env := output.NewEnvelope("TestKind", "test-cmd")
	env.SetStatus(2, 1)

	g.Expect(env.Status).ToNot(BeNil())
	g.Expect(env.Status.Result).To(Equal(output.StatusFailure))
	g.Expect(env.Status.Warnings).To(Equal(2))
	g.Expect(env.Status.Errors).To(Equal(1))
}
