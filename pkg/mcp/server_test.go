//nolint:testpackage // Tests internal server fields
package mcp

import (
	"testing"

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
