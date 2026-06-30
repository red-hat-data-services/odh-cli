//nolint:testpackage // Tests internal adapter functions
package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	pkgcmd "github.com/opendatahub-io/odh-cli/pkg/cmd"

	. "github.com/onsi/gomega"
)

// mockCommand implements pkgcmd.Command for testing the adapter.
type mockCommand struct {
	completeErr error
	validateErr error
	runErr      error
	runFunc     func(streams genericiooptions.IOStreams)
	streams     genericiooptions.IOStreams
}

var _ pkgcmd.Command = (*mockCommand)(nil)

func (m *mockCommand) AddFlags(_ *pflag.FlagSet) {}
func (m *mockCommand) Complete() error           { return m.completeErr }
func (m *mockCommand) Validate() error           { return m.validateErr }
func (m *mockCommand) Run(_ context.Context) error {
	if m.runFunc != nil {
		m.runFunc(m.streams)
	}

	return m.runErr
}

func newMockRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestToolAdapterHandle(t *testing.T) {
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should return stdout on successful run", func(t *testing.T) {
		g := NewWithT(t)

		adapter := &toolAdapter{
			configFlags: flags,
			factory: func(streams genericiooptions.IOStreams, _ *genericclioptions.ConfigFlags) pkgcmd.Command {
				return &mockCommand{
					streams: streams,
					runFunc: func(s genericiooptions.IOStreams) {
						_, _ = fmt.Fprint(s.Out, `{"status":"ok"}`)
					},
				}
			},
			applier: func(_ pkgcmd.Command, _ mcp.CallToolRequest) error { return nil },
		}

		result, err := adapter.handle(t.Context(), newMockRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.IsError).To(BeFalse())

		text := result.Content[0].(mcp.TextContent).Text
		g.Expect(text).To(Equal(`{"status":"ok"}`))
	})

	t.Run("should not leak stderr when stdout is empty", func(t *testing.T) {
		g := NewWithT(t)

		adapter := &toolAdapter{
			configFlags: flags,
			factory: func(streams genericiooptions.IOStreams, _ *genericclioptions.ConfigFlags) pkgcmd.Command {
				return &mockCommand{
					streams: streams,
					runFunc: func(s genericiooptions.IOStreams) {
						_, _ = fmt.Fprint(s.ErrOut, "progress info")
					},
				}
			},
			applier: func(_ pkgcmd.Command, _ mcp.CallToolRequest) error { return nil },
		}

		result, err := adapter.handle(t.Context(), newMockRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.IsError).To(BeFalse())

		text := result.Content[0].(mcp.TextContent).Text
		g.Expect(text).To(Equal("{}"))
	})

	t.Run("should return empty JSON when no output", func(t *testing.T) {
		g := NewWithT(t)

		adapter := &toolAdapter{
			configFlags: flags,
			factory: func(streams genericiooptions.IOStreams, _ *genericclioptions.ConfigFlags) pkgcmd.Command {
				return &mockCommand{streams: streams}
			},
			applier: func(_ pkgcmd.Command, _ mcp.CallToolRequest) error { return nil },
		}

		result, err := adapter.handle(t.Context(), newMockRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())

		text := result.Content[0].(mcp.TextContent).Text
		g.Expect(text).To(Equal("{}"))
	})

	t.Run("should return error result when applier fails", func(t *testing.T) {
		g := NewWithT(t)

		adapter := &toolAdapter{
			configFlags: flags,
			factory: func(streams genericiooptions.IOStreams, _ *genericclioptions.ConfigFlags) pkgcmd.Command {
				return &mockCommand{streams: streams}
			},
			applier: func(_ pkgcmd.Command, _ mcp.CallToolRequest) error {
				return errors.New("bad arguments")
			},
		}

		result, err := adapter.handle(t.Context(), newMockRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.IsError).To(BeTrue())
	})

	t.Run("should return error result when Complete fails", func(t *testing.T) {
		g := NewWithT(t)

		adapter := &toolAdapter{
			configFlags: flags,
			factory: func(streams genericiooptions.IOStreams, _ *genericclioptions.ConfigFlags) pkgcmd.Command {
				return &mockCommand{
					streams:     streams,
					completeErr: errors.New("failed to create client"),
				}
			},
			applier: func(_ pkgcmd.Command, _ mcp.CallToolRequest) error { return nil },
		}

		result, err := adapter.handle(t.Context(), newMockRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.IsError).To(BeTrue())
	})

	t.Run("should return error result when Validate fails", func(t *testing.T) {
		g := NewWithT(t)

		adapter := &toolAdapter{
			configFlags: flags,
			factory: func(streams genericiooptions.IOStreams, _ *genericclioptions.ConfigFlags) pkgcmd.Command {
				return &mockCommand{
					streams:     streams,
					validateErr: errors.New("invalid output format"),
				}
			},
			applier: func(_ pkgcmd.Command, _ mcp.CallToolRequest) error { return nil },
		}

		result, err := adapter.handle(t.Context(), newMockRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.IsError).To(BeTrue())
	})

	t.Run("should return error result when Run fails even with partial output", func(t *testing.T) {
		g := NewWithT(t)

		adapter := &toolAdapter{
			configFlags: flags,
			factory: func(streams genericiooptions.IOStreams, _ *genericclioptions.ConfigFlags) pkgcmd.Command {
				return &mockCommand{
					streams: streams,
					runFunc: func(s genericiooptions.IOStreams) {
						_, _ = fmt.Fprint(s.Out, `{"partial":"data"}`)
					},
					runErr: errors.New("partial failure"),
				}
			},
			applier: func(_ pkgcmd.Command, _ mcp.CallToolRequest) error { return nil },
		}

		result, err := adapter.handle(t.Context(), newMockRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.IsError).To(BeTrue())
	})

	t.Run("should return error result when Run fails with no output", func(t *testing.T) {
		g := NewWithT(t)

		adapter := &toolAdapter{
			configFlags: flags,
			factory: func(streams genericiooptions.IOStreams, _ *genericclioptions.ConfigFlags) pkgcmd.Command {
				return &mockCommand{
					streams: streams,
					runErr:  errors.New("total failure"),
				}
			},
			applier: func(_ pkgcmd.Command, _ mcp.CallToolRequest) error { return nil },
		}

		result, err := adapter.handle(t.Context(), newMockRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.IsError).To(BeTrue())
	})
}

func TestCloneConfigFlags(t *testing.T) {
	t.Run("should copy all pointer fields", func(t *testing.T) {
		g := NewWithT(t)

		kubeconfig := "/home/user/.kube/config"
		ctx := "my-context"
		ns := "test-ns"
		token := "my-token"

		src := genericclioptions.NewConfigFlags(true)
		src.KubeConfig = &kubeconfig
		src.Context = &ctx
		src.Namespace = &ns
		src.BearerToken = &token

		dst := cloneConfigFlags(src)

		g.Expect(dst.KubeConfig).ToNot(BeNil())
		g.Expect(*dst.KubeConfig).To(Equal(kubeconfig))
		g.Expect(dst.Context).ToNot(BeNil())
		g.Expect(*dst.Context).To(Equal(ctx))
		g.Expect(dst.Namespace).ToNot(BeNil())
		g.Expect(*dst.Namespace).To(Equal(ns))
		g.Expect(dst.BearerToken).ToNot(BeNil())
		g.Expect(*dst.BearerToken).To(Equal(token))
	})

	t.Run("should return independent struct", func(t *testing.T) {
		g := NewWithT(t)

		src := genericclioptions.NewConfigFlags(true)
		dst := cloneConfigFlags(src)

		g.Expect(dst).ToNot(BeIdenticalTo(src), "clone should be a different struct instance")
	})
}
