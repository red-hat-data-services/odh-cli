package cmd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/opendatahub-io/odh-cli/pkg/api"
)

const (
	DefaultPollInterval = 10 * time.Second
	MaxPollInterval     = 5 * time.Minute
	MaxWaitTimeout      = 30 * time.Minute

	flagDescWaitFor      = `Wait for a condition before exiting`
	flagDescPollInterval = "Poll interval for --wait-for (lower values increase API server load)"
)

// WaitCondition is a function that polls and returns (done, error).
type WaitCondition func(ctx context.Context) (done bool, err error)

// enumValue implements pflag.Value for a string constrained to a fixed set.
type enumValue struct {
	allowed []string
	value   *string
}

func newEnumValue(allowed []string, target *string) *enumValue {
	return &enumValue{allowed: allowed, value: target}
}

func (e *enumValue) String() string { return *e.value }

func (e *enumValue) Set(s string) error {
	if !slices.Contains(e.allowed, s) {
		return fmt.Errorf("must be one of: %s", strings.Join(e.allowed, ", "))
	}

	*e.value = s

	return nil
}

func (e *enumValue) Type() string {
	return strings.Join(e.allowed, "|")
}

// WaitOptions provides reusable --wait-for and --poll-interval flag
// support. Commands embed this struct and call its methods from their
// own AddFlags, Validate, and Run implementations.
type WaitOptions struct {
	WaitFor      string
	PollInterval time.Duration
}

// AddWaitFlags registers --wait-for and --poll-interval flags.
// validConditions lists the values accepted by --wait-for; invalid
// values are rejected at parse time by the enum flag type.
func (w *WaitOptions) AddWaitFlags(fs *pflag.FlagSet, validConditions []string) {
	fs.Var(newEnumValue(validConditions, &w.WaitFor), "wait-for", flagDescWaitFor)
	_ = fs.SetAnnotation("wait-for", api.AnnotationValidValues, validConditions)
	fs.DurationVar(&w.PollInterval, "poll-interval", w.PollInterval, flagDescPollInterval)
}

// HasWaitMode returns true if --wait-for is active.
func (w *WaitOptions) HasWaitMode() bool {
	return w.WaitFor != ""
}

// ValidateWait checks that --poll-interval and timeout are within bounds.
// Condition validation is handled at parse time by the enum flag type.
// timeout is the caller's --timeout value (0 means no timeout in wait mode).
func (w *WaitOptions) ValidateWait(timeout time.Duration) error {
	if w.WaitFor == "" {
		return nil
	}

	if w.PollInterval < time.Second || w.PollInterval > MaxPollInterval {
		return ErrInvalidPollInterval()
	}

	if timeout < 0 || timeout > MaxWaitTimeout {
		return ErrInvalidWaitTimeout()
	}

	return nil
}

// RunWait polls cond at the configured interval until it returns true,
// the timeout expires, or the context is cancelled. Pass timeout=0 for
// no timeout (poll until condition or cancellation).
func (w *WaitOptions) RunWait(ctx context.Context, timeout time.Duration, cond WaitCondition) error {
	if w.PollInterval < time.Second || w.PollInterval > MaxPollInterval {
		return ErrInvalidPollInterval()
	}

	if timeout < 0 {
		return ErrInvalidWaitTimeout()
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)

		defer cancel()
	}

	pollErr := wait.PollUntilContextCancel(ctx, w.PollInterval, true, wait.ConditionWithContextFunc(cond))

	if pollErr != nil {
		if errors.Is(pollErr, context.DeadlineExceeded) {
			return ErrWaitTimeout(w.WaitFor)
		}

		return fmt.Errorf("waiting for condition %q: %w", w.WaitFor, pollErr)
	}

	return nil
}
