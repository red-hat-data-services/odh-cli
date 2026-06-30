package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/internal/version"
)

// Transport identifies the MCP transport protocol.
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportSSE   Transport = "sse"
)

// Server wraps an MCP server with transport configuration.
type Server struct {
	configFlags *genericclioptions.ConfigFlags
	transport   Transport
	port        int
	mcpServer   *server.MCPServer
}

// NewServer creates a new MCP server.
// Call registerTools to add tool definitions before calling Serve.
func NewServer(configFlags *genericclioptions.ConfigFlags, transport Transport, port int) *Server {
	mcpServer := server.NewMCPServer(
		"odh-cli",
		version.GetVersion(),
	)

	return &Server{
		configFlags: configFlags,
		transport:   transport,
		port:        port,
		mcpServer:   mcpServer,
	}
}

func (s *Server) registerTools(tools []toolDefinition) { //nolint:unused // used by tools.go in a follow-up PR
	for _, def := range tools {
		s.mcpServer.AddTool(def.tool, def.handler)
	}
}

// Serve starts the MCP server with the configured transport.
// It blocks until the context is cancelled or the transport shuts down.
func (s *Server) Serve(ctx context.Context) error {
	switch s.transport {
	case TransportStdio:
		stdioServer := server.NewStdioServer(s.mcpServer)

		if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
			return fmt.Errorf("stdio transport: %w", err)
		}

		return nil
	case TransportSSE:
		return s.serveSSE(ctx)
	default:
		return fmt.Errorf("unsupported transport: %s", s.transport)
	}
}

const sseShutdownTimeout = 5 * time.Second

func (s *Server) serveSSE(ctx context.Context) error {
	sseServer := server.NewSSEServer(s.mcpServer)
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)

	errCh := make(chan error, 1)

	go func() {
		errCh <- sseServer.Start(addr)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("SSE transport: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), sseShutdownTimeout)
		defer cancel()

		if err := sseServer.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck // new context needed for graceful shutdown after parent cancellation
			return fmt.Errorf("SSE shutdown: %w", err)
		}

		return nil
	}
}
