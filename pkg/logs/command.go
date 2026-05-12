package logs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

const (
	targetOperator = "operator"

	// componentLabelSelector finds component pods by their part-of label.
	// The label value must match the component key in resources.ComponentCRResourceTypes
	// (e.g., "dashboard", "kserve", "ray"). This contract is set by the ODH/RHOAI operator.
	componentLabelSelector = "app.kubernetes.io/part-of=%s"

	flagDescFollow    = "Follow log output (stream new logs as they appear)"
	flagDescTail      = "Lines of recent log file to display (default: all)"
	flagDescSince     = "Only return logs newer than a relative duration (e.g., 5s, 2m, 3h)"
	flagDescPrevious  = "Print the logs for the previous instance of the container (for crash debugging)"
	flagDescContainer = "Container name (if pod has multiple containers)"

	logChannelBuffer = 100

	// Scanner buffer sizes for handling long JSON log lines.
	scannerInitialBuffer = 64 * 1024        // 64 KB initial
	scannerMaxBuffer     = 10 * 1024 * 1024 // 10 MB max

	errMsgUnsupportedTarget   = "unsupported target %q, supported targets: %s"
	errMsgNoOperatorPod       = "no running operator pod found; checked namespaces: %v"
	errMsgNoOperatorPodErrs   = "no running operator pod found; errors: %v"
	errMsgOpenLogStream       = "opening log stream for pod %s/%s: %w"
	errMsgReadingLogs         = "reading logs: %w"
	errMsgScanningLogs        = "scanning logs: %w"
	errMsgStreamingLogs       = "streaming logs: %w"
	errMsgCreatingClient      = "creating client: %w"
	errMsgDiscoveringPods     = "discovering operator pods: %w"
	errMsgNamespaceListError  = "namespace %s: %w"
	errMsgWritingOutput       = "writing output: %w"
	errMsgNoComponentPod      = "no running pods found for component %q in namespace %q"
	errMsgGettingAppNamespace = "getting applications namespace: %w"
	errMsgListingComponentPod = "listing pods for component %s: %w"

	suggestionNoOperatorPod  = "Verify the ODH/RHOAI operator is installed and running"
	suggestionNoComponentPod = "Check if the component is enabled in your DataScienceCluster"
	suggestionInvalidTarget  = "Use 'operator' or a valid component name (dashboard, kserve, ray, etc.)"
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

// componentLabelOverrides maps CLI target names to actual part-of label values
// for components where the label differs from the target name.
//
//nolint:gochecknoglobals // Static configuration for label discovery
var componentLabelOverrides = map[string]string{
	"aipipelines":   "data-science-pipelines-operator",
	"modelregistry": "model-registry-operator",
}

// ValidTargets contains sorted list of valid log targets (operator + components).
// Built once at init time and reused by Validate() and shell completion.
//
//nolint:gochecknoglobals // Static configuration built once at init
var ValidTargets []string

//nolint:gochecknoinits // One-time initialization of static target list
func init() {
	ValidTargets = make([]string, 0, 1+len(resources.ComponentCRResourceTypes))
	ValidTargets = append(ValidTargets, targetOperator)

	for name := range resources.ComponentCRResourceTypes {
		ValidTargets = append(ValidTargets, name)
	}

	sort.Strings(ValidTargets[1:]) // Sort components, keep operator first
}

// Command implements the logs command.
type Command struct {
	IO     iostreams.Interface
	Flags  *genericclioptions.ConfigFlags
	Client client.Client

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
		IO:    iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		Flags: flags,
		Tail:  -1,
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
	if c.Target == targetOperator {
		return nil
	}

	// Check if target is a valid component
	if resources.GetComponentCR(c.Target) != nil {
		return nil
	}

	return clierrors.NewValidationError(
		"INVALID_TARGET",
		fmt.Sprintf(errMsgUnsupportedTarget, c.Target, strings.Join(ValidTargets, ", ")),
		suggestionInvalidTarget,
	)
}

// Run executes the logs command.
func (c *Command) Run(ctx context.Context) error {
	var pods []*corev1.Pod
	var err error

	if c.Target == targetOperator {
		pods, err = c.discoverOperatorPods(ctx)
	} else {
		pods, err = c.discoverComponentPods(ctx)
	}

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
			return nil, clierrors.NewValidationError(
				"NO_OPERATOR_POD",
				fmt.Sprintf(errMsgNoOperatorPodErrs, errs),
				suggestionNoOperatorPod,
			)
		}

		return nil, clierrors.NewValidationError(
			"NO_OPERATOR_POD",
			fmt.Sprintf(errMsgNoOperatorPod, operatorNamespaces),
			suggestionNoOperatorPod,
		)
	}

	return result, nil
}

// getComponentLabelValue returns the part-of label value for a component.
// Some components have labels that differ from the CLI target name.
func (c *Command) getComponentLabelValue() string {
	if override, ok := componentLabelOverrides[c.Target]; ok {
		return override
	}

	return c.Target
}

// discoverComponentPods finds pods for a component using the applications namespace.
func (c *Command) discoverComponentPods(ctx context.Context) ([]*corev1.Pod, error) {
	// Get applications namespace where components are deployed
	ns, err := client.GetApplicationsNamespace(ctx, c.Client)
	if err != nil {
		return nil, fmt.Errorf(errMsgGettingAppNamespace, err)
	}

	// List pods by component label (use override if target name differs from label)
	labelValue := c.getComponentLabelValue()
	selector := fmt.Sprintf(componentLabelSelector, labelValue)

	pods, err := c.Client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return nil, fmt.Errorf(errMsgListingComponentPod, c.Target, err)
	}

	if len(pods.Items) == 0 {
		return nil, clierrors.NewValidationError(
			"NO_COMPONENT_POD",
			fmt.Sprintf(errMsgNoComponentPod, c.Target, ns),
			suggestionNoComponentPod,
		)
	}

	result := make([]*corev1.Pod, len(pods.Items))
	for i := range pods.Items {
		result[i] = &pods.Items[i]
	}

	return result, nil
}

// podContainer pairs a pod with a specific container for streaming.
type podContainer struct {
	Pod       *corev1.Pod
	Container string
	Prefix    string
}

// streamLogs streams logs from discovered pods.
func (c *Command) streamLogs(ctx context.Context) error {
	// Validate -c flag against all pods before streaming
	if c.Container != "" {
		for _, pod := range c.Pods {
			found := false

			for _, ctr := range pod.Spec.Containers {
				if ctr.Name == c.Container {
					found = true

					break
				}
			}

			if !found {
				return clierrors.NewValidationError(
					"INVALID_CONTAINER",
					fmt.Sprintf("container %q not found in pod %q", c.Container, pod.Name),
					"Use -c with a container name present in all selected pods",
				)
			}
		}
	}

	// Build list of pod/container pairs to stream
	targets := c.buildStreamTargets()

	opts := &corev1.PodLogOptions{
		Follow:   c.Follow,
		Previous: c.Previous,
	}

	if c.Tail >= 0 {
		opts.TailLines = &c.Tail
	}

	if c.Since > 0 {
		seconds := int64(c.Since.Seconds())
		opts.SinceSeconds = &seconds
	}

	// Single target: stream directly without prefix
	if len(targets) == 1 {
		opts.Container = targets[0].Container

		return c.streamSinglePod(ctx, targets[0].Pod, opts)
	}

	// Multiple targets: stream with prefixes
	return c.streamMultipleTargets(ctx, targets, opts)
}

// buildStreamTargets expands pods into pod/container pairs with computed prefixes.
// If -c is specified, uses that container for all pods.
// Otherwise, streams all containers from each pod.
func (c *Command) buildStreamTargets() []podContainer {
	var targets []podContainer

	// First pass: collect all targets
	for _, pod := range c.Pods {
		if c.Container != "" {
			targets = append(targets, podContainer{Pod: pod, Container: c.Container})
		} else if len(pod.Spec.Containers) == 1 {
			targets = append(targets, podContainer{Pod: pod, Container: pod.Spec.Containers[0].Name})
		} else {
			for _, container := range pod.Spec.Containers {
				targets = append(targets, podContainer{Pod: pod, Container: container.Name})
			}
		}
	}

	// Second pass: compute prefixes based on total target count
	multiplePods := len(c.Pods) > 1

	for i := range targets {
		if multiplePods {
			targets[i].Prefix = fmt.Sprintf("[%s/%s] ", targets[i].Pod.Name, targets[i].Container)
		} else {
			targets[i].Prefix = fmt.Sprintf("[%s] ", targets[i].Container)
		}
	}

	return targets
}

// streamSinglePod streams logs from a single pod without prefix.
func (c *Command) streamSinglePod(ctx context.Context, pod *corev1.Pod, opts *corev1.PodLogOptions) error {
	req := c.Client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, opts)

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf(errMsgOpenLogStream, pod.Namespace, pod.Name, err)
	}
	defer func() { _ = stream.Close() }()

	_, err = io.Copy(c.IO.Out(), stream)
	if err != nil {
		return fmt.Errorf(errMsgReadingLogs, err)
	}

	return nil
}

// streamMultipleTargets streams logs from multiple pod/container pairs with prefix.
// Uses channel-based fan-in for better throughput under heavy log volume.
func (c *Command) streamMultipleTargets(ctx context.Context, targets []podContainer, opts *corev1.PodLogOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lines := make(chan string, logChannelBuffer)

	var wg sync.WaitGroup

	var mu sync.Mutex

	var firstErr error

	// Start producer goroutines
	for _, target := range targets {
		wg.Add(1)

		go func(t podContainer) {
			defer wg.Done()

			if err := c.streamTargetToChannel(ctx, t, opts, lines); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel() // Signal other producers to stop on first error
				}
				mu.Unlock()
			}
		}(target)
	}

	// Close channel when all producers are done
	go func() {
		wg.Wait()
		close(lines)
	}()

	// Single writer consumes from channel
	for line := range lines {
		if _, err := fmt.Fprintln(c.IO.Out(), line); err != nil {
			cancel() // Signal producers to stop

			return fmt.Errorf(errMsgWritingOutput, err)
		}
	}

	return firstErr
}

// streamTargetToChannel streams logs from a pod/container to a channel, prefixing each line.
func (c *Command) streamTargetToChannel(ctx context.Context, target podContainer, opts *corev1.PodLogOptions, lines chan<- string) error {
	targetOpts := *opts
	targetOpts.Container = target.Container

	req := c.Client.CoreV1().Pods(target.Pod.Namespace).GetLogs(target.Pod.Name, &targetOpts)

	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf(errMsgOpenLogStream, target.Pod.Namespace, target.Pod.Name, err)
	}
	defer func() { _ = stream.Close() }()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, scannerInitialBuffer), scannerMaxBuffer)

	for scanner.Scan() {
		select {
		case lines <- target.Prefix + scanner.Text():
		case <-ctx.Done():
			return fmt.Errorf(errMsgStreamingLogs, ctx.Err())
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf(errMsgScanningLogs, err)
	}

	return nil
}
