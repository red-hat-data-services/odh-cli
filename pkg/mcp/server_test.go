//nolint:testpackage // Tests internal server fields
package mcp

import (
	"path/filepath"
	"testing"

	mcpgoserver "github.com/mark3labs/mcp-go/server"
	"github.com/opendatahub-io/opendatahub-operator/pkg/mcptools"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	. "github.com/onsi/gomega"
)

func TestNewServer(t *testing.T) {
	t.Run("should create server with all fields set", func(t *testing.T) {
		g := NewWithT(t)

		flags := genericclioptions.NewConfigFlags(true)
		srv := NewServer(flags, TransportStdio, 8080)

		g.Expect(srv).ToNot(BeNil())
		g.Expect(srv.mcpServer).ToNot(BeNil())
		g.Expect(srv.transport).To(Equal(TransportStdio))
		g.Expect(srv.port).To(Equal(8080))
	})
}

func TestNewServerRegistersAllTools(t *testing.T) {
	t.Run("should register all 12 tools on the MCP server", func(t *testing.T) {
		g := NewWithT(t)

		flags := genericclioptions.NewConfigFlags(true)
		// Point to a nonexistent kubeconfig so diagnostic tools are skipped and exactly 12 CLI tools are registered.
		noKubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
		flags.KubeConfig = &noKubeconfig
		srv := NewServer(flags, TransportStdio, 8080)

		registered := srv.mcpServer.ListTools()
		g.Expect(registered).To(HaveLen(12))

		expectedTools := []string{
			"odh_status", "odh_lint", "odh_get", "odh_deps", "odh_deps_install",
			"odh_backup", "odh_logs", "odh_events",
			"odh_components_list", "odh_components_describe",
			"odh_migrate_list", "odh_migrate_run",
		}
		for _, name := range expectedTools {
			g.Expect(registered).To(HaveKey(name), "tool %s should be registered", name)
		}
	})
}

func TestRegisterDiagnosticToolsRegistersAll5(t *testing.T) {
	t.Run("registers exactly 5 diagnostic tools when a kube client is available", func(t *testing.T) {
		g := NewWithT(t)

		mcpSrv := mcpgoserver.NewMCPServer("odh-cli", "test")
		fakeClient := fake.NewClientBuilder().Build()
		mcptools.RegisterAll(mcpSrv, fakeClient)

		registered := mcpSrv.ListTools()
		g.Expect(registered).To(HaveLen(5))

		diagnosticTools := []string{
			"platform_health", "classify_failure", "component_status",
			"recent_events", "operator_dependencies",
		}
		for _, name := range diagnosticTools {
			g.Expect(registered).To(HaveKey(name), "diagnostic tool %s should be registered", name)
		}
	})
}

func TestServeUnsupportedTransport(t *testing.T) {
	t.Run("should return error for unsupported transport", func(t *testing.T) {
		g := NewWithT(t)

		flags := genericclioptions.NewConfigFlags(true)
		srv := NewServer(flags, Transport("grpc"), 8080)

		err := srv.Serve(t.Context())

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("unsupported transport"))
	})
}
