package logs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

const (
	targetOperator = "operator"

	flagDescFollow    = "Follow log output (stream new logs as they appear)"
	flagDescTail      = "Lines of recent log file to display (default: all)"
	flagDescSince     = "Only return logs newer than a relative duration (e.g., 5s, 2m, 3h)"
	flagDescPrevious  = "Print the logs for the previous instance of the container (for crash debugging)"
	flagDescContainer = "Container name (if pod has multiple containers)"

	logChannelBuffer = 100

	// Scanner buffer sizes for handling long JSON log lines.
	scannerInitialBuffer = 64 * 1024        // 64 KB initial
	scannerMaxBuffer     = 10 * 1024 * 1024 // 10 MB max

	errMsgUnsupportedTarget  = "unsupported target %q, supported targets: %s"
	errMsgNoOperatorPod      = "no running operator pod found; checked namespaces: %v"
	errMsgNoOperatorPodErrs  = "no running operator pod found; errors: %v"
	errMsgOpenLogStream      = "opening log stream for pod %s/%s: %w"
	errMsgReadingLogs        = "reading logs: %w"
	errMsgScanningLogs       = "scanning logs: %w"
	errMsgStreamingLogs      = "streaming logs: %w"
	errMsgCreatingClient     = "creating client: %w"
	errMsgDiscoveringPods    = "discovering operator pods: %w"
	errMsgNamespaceListError = "namespace %s: %w"
	errMsgWritingOutput      = "writing output: %w"
)

// operatorLabelSelector finds ODH/RHOAI operator pods using a single query.
// Uses "in" operator to match both ODH and RHOAI operator names.
const operatorLabelSelector = "control-plane=controller-manager,name in (opendatahub-operator,rhods-operator)"

// operatorNamespaces contains namespaces where the operator might be installed.
//
//nolint:gochecknoglobals // Static configuration for operator discovery
var operatorNamespaces = []string{
	"openshift-operators",
	"redhat-ods-operator",
	"opendatahub-operator-system",
}

// Command implements the logs command.
type Command struct {
	Streams genericiooptions.IOStreams
	Flags   *genericclioptions.ConfigFlags
	Client  client.Client

	// Args
	Target string

	// Flags
	Follow    bool
	Tail      int64
	Since     time.Duration
	Previous  bool
	Container string

	// Resolved state
	Pods []*corev1.Pod
}

// NewCommand creates a new logs command.
func NewCommand(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) *Command {
	return &Command{
		Streams: streams,
		Flags:   flags,
		Tail:    -1,
	}
}

// AddFlags adds command flags.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVarP(&c.Follow, "follow", "f", false, flagDescFollow)
	fs.Int64Var(&c.Tail, "tail", -1, flagDescTail)
	fs.DurationVar(&c.Since, "since", 0, flagDescSince)
	fs.BoolVar(&c.Previous, "previous", false, flagDescPrevious)
	fs.StringVarP(&c.Container, "container", "c", "", flagDescContainer)
}

// Complete initializes the command state.
func (c *Command) Complete() error {
	var err error

	c.Client, err = client.NewClient(c.Flags)
	if err != nil {
		return fmt.Errorf(errMsgCreatingClient, err)
	}

	return nil
}

// Validate checks command arguments and flags.
func (c *Command) Validate() error {
	if c.Target != targetOperator {
		return fmt.Errorf(errMsgUnsupportedTarget, c.Target, targetOperator)
	}

	return nil
}

// Run executes the logs command.
func (c *Command) Run(ctx context.Context) error {
	pods, err := c.discoverOperatorPods(ctx)
	if err != nil {
		return err
	}

	c.Pods = pods

	return c.streamLogs(ctx)
}

// discoverOperatorPods finds all ODH/RHOAI operator pods concurrently across namespaces.
func (c *Command) discoverOperatorPods(ctx context.Context) ([]*corev1.Pod, error) {
	coreClient := c.Client.CoreV1()

	var (
		result []*corev1.Pod
		errs   []error
		mu     sync.Mutex
	)

	g, ctx := errgroup.WithContext(ctx)

	for _, ns := range operatorNamespaces {
		g.Go(func() error {
			pods, err := coreClient.Pods(ns).List(ctx, metav1.ListOptions{
				LabelSelector: operatorLabelSelector,
				FieldSelector: "status.phase=Running",
			})
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf(errMsgNamespaceListError, ns, err))
				mu.Unlock()

				return nil // Continue checking other namespaces
			}

			mu.Lock()
			for i := range pods.Items {
				result = append(result, &pods.Items[i])
			}
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf(errMsgDiscoveringPods, err)
	}

	if len(result) == 0 {
		if len(errs) > 0 {
			return nil, fmt.Errorf(errMsgNoOperatorPodErrs, errs)
		}

		return nil, fmt.Errorf(errMsgNoOperatorPod, operatorNamespaces)
	}

	return result, nil
}

// streamLogs streams logs from discovered pods.
func (c *Command) streamLogs(ctx context.Context) error {
	opts := &corev1.PodLogOptions{
		Follow:    c.Follow,
		Previous:  c.Previous,
		Container: c.Container,
	}

	if c.Tail >= 0 {
		opts.TailLines = &c.Tail
	}

	if c.Since > 0 {
		seconds := int64(c.Since.Seconds())
		opts.SinceSeconds = &seconds
	}

	// Single pod: stream directly without prefix
	if len(c.Pods) == 1 {
		return c.streamSinglePod(ctx, c.Pods[0], opts)
	}

	// Multiple pods: stream with prefixes
	return c.streamMultiplePods(ctx, opts)
}

// streamSinglePod streams logs from a single pod without prefix.
func (c *Command) streamSinglePod(ctx context.Context, pod *corev1.Pod, opts *corev1.PodLogOptions) error {
	req := c.Client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts)

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf(errMsgOpenLogStream, pod.Namespace, pod.Name, err)
	}
	defer func() { _ = stream.Close() }()

	_, err = io.Copy(c.Streams.Out, stream)
	if err != nil {
		return fmt.Errorf(errMsgReadingLogs, err)
	}

	return nil
}

// streamMultiplePods streams logs from multiple pods with [pod-name] prefix.
// Uses channel-based fan-in for better throughput under heavy log volume.
func (c *Command) streamMultiplePods(ctx context.Context, opts *corev1.PodLogOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lines := make(chan string, logChannelBuffer)

	var wg sync.WaitGroup

	var mu sync.Mutex

	var firstErr error

	// Start producer goroutines
	for _, pod := range c.Pods {
		wg.Add(1)

		go func(pod *corev1.Pod) {
			defer wg.Done()

			if err := c.streamPodToChannel(ctx, pod, opts, lines); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(pod)
	}

	// Close channel when all producers are done
	go func() {
		wg.Wait()
		close(lines)
	}()

	// Single writer consumes from channel
	for line := range lines {
		if _, err := fmt.Fprintln(c.Streams.Out, line); err != nil {
			cancel() // Signal producers to stop

			return fmt.Errorf(errMsgWritingOutput, err)
		}
	}

	return firstErr
}

// streamPodToChannel streams logs from a pod to a channel, prefixing each line.
func (c *Command) streamPodToChannel(ctx context.Context, pod *corev1.Pod, opts *corev1.PodLogOptions, lines chan<- string) error {
	req := c.Client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts)

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf(errMsgOpenLogStream, pod.Namespace, pod.Name, err)
	}
	defer func() { _ = stream.Close() }()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, scannerInitialBuffer), scannerMaxBuffer)
	prefix := fmt.Sprintf("[%s] ", pod.Name)

	for scanner.Scan() {
		select {
		case lines <- prefix + scanner.Text():
		case <-ctx.Done():
			return fmt.Errorf(errMsgStreamingLogs, ctx.Err())
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf(errMsgScanningLogs, err)
	}

	return nil
}
