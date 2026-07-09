package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

const (
	MaxWatchErrors      = 5
	InitialWatchBackoff = time.Second
	MaxWatchBackoff     = 30 * time.Second
	WatchBackoffFactor  = 2

	flagDescWatch = "Watch for changes (streaming JSON or YAML output)"

	msgWatchRetry = "Warning: poll failed (retrying): %v\n"
)

// WatchPoller polls for new data.
// Returns the data to emit and a snapshot for state-change comparison.
type WatchPoller func(ctx context.Context) (data any, snapshot []byte, err error)

// WatchEmitter writes one streaming output item.
type WatchEmitter func(data any) error

// WatchRunConfig configures a RunWatch invocation.
type WatchRunConfig struct {
	Timeout      time.Duration
	PollInterval time.Duration
	Poller       WatchPoller
	Emitter      WatchEmitter
	ErrOut       io.Writer
}

// WatchOptions provides a reusable --watch flag.
// Commands embed this struct and call its methods from their
// own AddFlags, Validate, and Run implementations.
type WatchOptions struct {
	Watch bool
}

// AddWatchFlag registers the --watch flag.
func (w *WatchOptions) AddWatchFlag(fs *pflag.FlagSet) {
	fs.BoolVar(&w.Watch, "watch", false, flagDescWatch)
}

// ValidateWatch checks that --watch is consistent with other flags.
// waitActive is true if --wait-for is set (mutually exclusive).
// outputFormat is the current -o value; structuredFormats lists the
// formats that support streaming (e.g., ["json", "yaml"]).
func (w *WatchOptions) ValidateWatch(
	waitActive bool,
	outputFormat string,
	structuredFormats []string,
	timeout, pollInterval time.Duration,
) error {
	if !w.Watch {
		return nil
	}

	if waitActive {
		return ErrWatchWaitExclusive()
	}

	if !slices.Contains(structuredFormats, outputFormat) {
		return ErrWatchRequiresStructuredOutput()
	}

	if pollInterval < time.Second || pollInterval > MaxPollInterval {
		return ErrInvalidPollInterval()
	}

	if timeout < 0 || timeout > MaxWaitTimeout {
		return ErrInvalidWaitTimeout()
	}

	return nil
}

// RunWatch polls via cfg.Poller and emits via cfg.Emitter on each state change.
// It runs until the context is cancelled, the timeout expires, or an
// unrecoverable error occurs. Transient errors are retried with exponential
// backoff up to MaxWatchErrors consecutive failures.
func (w *WatchOptions) RunWatch(ctx context.Context, cfg WatchRunConfig) error {
	if cfg.PollInterval < time.Second || cfg.PollInterval > MaxPollInterval {
		return ErrInvalidPollInterval()
	}

	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)

		defer cancel()
	}

	data, snapshot, err := cfg.Poller(ctx)
	if err != nil {
		return err
	}

	if err := cfg.Emitter(data); err != nil {
		return err
	}

	prevSnapshot := snapshot

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	ws := &watchState{backoff: InitialWatchBackoff}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			data, snapshot, err = cfg.Poller(ctx)
			if err != nil {
				if done, retErr := ws.handleError(ctx, err, cfg.ErrOut); done {
					return retErr
				}

				continue
			}

			ws.reset()

			if bytes.Equal(prevSnapshot, snapshot) {
				continue
			}

			prevSnapshot = snapshot

			if err := cfg.Emitter(data); err != nil {
				return err
			}
		}
	}
}

type watchState struct {
	consecutiveErrors int
	backoff           time.Duration
}

// handleError processes a transient poll error with backoff.
// Returns (true, err) to stop the loop, (false, nil) to continue.
func (s *watchState) handleError(ctx context.Context, err error, errOut io.Writer) (bool, error) {
	if client.IsUnrecoverableError(err) {
		return true, err
	}

	s.consecutiveErrors++
	if s.consecutiveErrors >= MaxWatchErrors {
		return true, ErrWatchMaxErrors(err)
	}

	if errOut != nil {
		_, _ = fmt.Fprintf(errOut, msgWatchRetry, err)
	}

	select {
	case <-time.After(s.backoff):
	case <-ctx.Done():
		return true, nil
	}

	s.backoff = min(s.backoff*WatchBackoffFactor, MaxWatchBackoff)

	return false, nil
}

func (s *watchState) reset() {
	s.consecutiveErrors = 0
	s.backoff = InitialWatchBackoff
}
