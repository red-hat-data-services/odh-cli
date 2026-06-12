package exec

import (
	"context"
	"errors"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecOptions configures a single pod exec invocation.
type ExecOptions struct {
	Namespace     string
	PodName       string
	ContainerName string
	Command       []string
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
}

// Executor runs commands inside Kubernetes pods.
type Executor interface {
	Exec(ctx context.Context, opts ExecOptions) error
}

// spdyExecutor implements Executor using SPDY-based remote command execution.
type spdyExecutor struct {
	config *rest.Config
	client corev1client.CoreV1Interface
}

// NewSPDYExecutor creates an Executor that uses SPDY to stream commands to pods.
func NewSPDYExecutor(config *rest.Config, client corev1client.CoreV1Interface) Executor {
	return &spdyExecutor{
		config: config,
		client: client,
	}
}

func (e *spdyExecutor) Exec(ctx context.Context, opts ExecOptions) error {
	if e.client == nil {
		return errors.New("exec: client is nil")
	}
	if e.config == nil {
		return errors.New("exec: REST config is nil")
	}

	req := e.client.RESTClient().Post().
		Resource("pods").
		Name(opts.PodName).
		Namespace(opts.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: opts.ContainerName,
			Command:   opts.Command,
			Stdin:     opts.Stdin != nil,
			Stdout:    opts.Stdout != nil,
			Stderr:    opts.Stderr != nil,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(e.config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("creating SPDY executor: %w", err)
	}

	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  opts.Stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	}); err != nil {
		return fmt.Errorf("executing command in pod %s/%s: %w", opts.Namespace, opts.PodName, err)
	}

	return nil
}
