package status

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	cmdpkg "github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

const (
	watchTestPollInterval = 1 * time.Second
	watchTestTimeout      = 10 * time.Second
)

func newTestWatchCommand(cfg *clusterhealth.Config) (*Command, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer

	cmd := &Command{
		IO:           iostreams.NewIOStreams(nil, &stdout, &stderr),
		OutputFormat: OutputFormatJSON,
		WatchOptions: cmdpkg.WatchOptions{Watch: true},
		WaitOptions: cmdpkg.WaitOptions{
			PollInterval: watchTestPollInterval,
		},
		Timeout:      watchTestTimeout,
		healthConfig: cfg,
	}

	return cmd, &stdout, &stderr
}

func TestRunWatch(t *testing.T) {
	scheme := testScheme()

	t.Run("emits initial report immediately", func(t *testing.T) {
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())

		cmd, stdout, _ := newTestWatchCommand(healthyConfig(scheme))
		cmd.Timeout = 0

		go func() {
			time.Sleep(500 * time.Millisecond)
			cancel()
		}()

		err := cmd.runWatch(ctx)
		g.Expect(err).ToNot(HaveOccurred())

		lines := nonEmptyLines(stdout.String())
		g.Expect(lines).ToNot(BeEmpty())

		var parsed map[string]any
		g.Expect(json.Unmarshal([]byte(lines[0]), &parsed)).To(Succeed())
		g.Expect(parsed).To(HaveKey("report"))
	})

	t.Run("suppresses duplicate state", func(t *testing.T) {
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())

		cmd, stdout, _ := newTestWatchCommand(healthyConfig(scheme))
		cmd.Timeout = 0

		go func() {
			time.Sleep(3500 * time.Millisecond)
			cancel()
		}()

		err := cmd.runWatch(ctx)
		g.Expect(err).ToNot(HaveOccurred())

		lines := nonEmptyLines(stdout.String())
		g.Expect(lines).To(HaveLen(1))
	})

	t.Run("emits on state change", func(t *testing.T) {
		g := NewWithT(t)

		cfg := unhealthyConfig(scheme)
		cmd, stdout, _ := newTestWatchCommand(cfg)
		cmd.Timeout = 0

		dep := testOperatorDeployment()
		setupErr := make(chan error, 1)
		setupCtx := t.Context()
		stateChanged := make(chan struct{}, 1)

		go func() {
			time.Sleep(1500 * time.Millisecond)

			if err := cfg.Client.Create(setupCtx, dep); err != nil {
				setupErr <- err

				return
			}

			dep.Status.ReadyReplicas = 1
			dep.Status.Replicas = 1
			dep.Status.AvailableReplicas = 1
			setupErr <- cfg.Client.Status().Update(setupCtx, dep)
		}()

		ctx, cancel := context.WithCancel(t.Context())

		// Cancel after detecting the state change emission rather than
		// relying on wall-clock timing.
		origOut := cmd.IO.Out()
		cmd.IO = iostreams.NewIOStreams(nil, writerFunc(func(p []byte) (int, error) {
			n, err := origOut.Write(p)
			if err == nil {
				select {
				case stateChanged <- struct{}{}:
				default:
				}
			}

			return n, err //nolint:wrapcheck // test passthrough writer
		}), cmd.IO.ErrOut())

		go func() {
			// Wait for initial emit, then wait for the state-change emit.
			<-stateChanged // initial
			<-stateChanged // state change
			cancel()
		}()

		err := cmd.runWatch(ctx)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(<-setupErr).ToNot(HaveOccurred())

		lines := nonEmptyLines(stdout.String())
		g.Expect(len(lines)).To(BeNumerically(">=", 2))
	})

	t.Run("cancelled context exits cleanly", func(t *testing.T) {
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		cmd, _, _ := newTestWatchCommand(healthyConfig(scheme))
		cmd.Timeout = 0

		err := cmd.runWatch(ctx)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("yaml output produces multi-document format", func(t *testing.T) {
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())

		cmd, stdout, _ := newTestWatchCommand(healthyConfig(scheme))
		cmd.OutputFormat = OutputFormatYAML
		cmd.Timeout = 0

		go func() {
			time.Sleep(500 * time.Millisecond)
			cancel()
		}()

		err := cmd.runWatch(ctx)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(stdout.String()).To(HavePrefix("---\n"))
		g.Expect(stdout.String()).To(ContainSubstring("apiVersion:"))
	})
}

// testOperatorDeployment is defined in wait_internal_test.go but we need it here too.
// Since both files are in the same package, we can use it directly.

// writerFunc adapts a function to io.Writer for test instrumentation.
type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func nonEmptyLines(s string) []string {
	var lines []string

	for line := range strings.SplitSeq(strings.TrimSpace(s), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}
