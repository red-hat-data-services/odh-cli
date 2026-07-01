package cmd_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"

	. "github.com/onsi/gomega"
)

func TestValidateWait(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     cmd.WaitOptions
		timeout  time.Duration
		wantErr  bool
		wantCode string
	}{
		{
			name:    "empty wait-for passes",
			opts:    cmd.WaitOptions{},
			timeout: 30 * time.Second,
		},
		{
			name:    "valid condition passes",
			opts:    cmd.WaitOptions{WaitFor: "healthy", PollInterval: 10 * time.Second},
			timeout: 30 * time.Second,
		},
		{
			name:    "second valid condition passes",
			opts:    cmd.WaitOptions{WaitFor: "ready", PollInterval: 5 * time.Second},
			timeout: 60 * time.Second,
		},
		{
			name:    "zero timeout valid in wait mode",
			opts:    cmd.WaitOptions{WaitFor: "healthy", PollInterval: 10 * time.Second},
			timeout: 0,
		},
		{
			name:     "zero poll interval rejected",
			opts:     cmd.WaitOptions{WaitFor: "healthy", PollInterval: 0},
			timeout:  30 * time.Second,
			wantErr:  true,
			wantCode: "INVALID_POLL_INTERVAL",
		},
		{
			name:     "negative poll interval rejected",
			opts:     cmd.WaitOptions{WaitFor: "healthy", PollInterval: -1 * time.Second},
			timeout:  30 * time.Second,
			wantErr:  true,
			wantCode: "INVALID_POLL_INTERVAL",
		},
		{
			name:     "negative timeout rejected",
			opts:     cmd.WaitOptions{WaitFor: "healthy", PollInterval: 10 * time.Second},
			timeout:  -1 * time.Second,
			wantErr:  true,
			wantCode: "INVALID_TIMEOUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			err := tt.opts.ValidateWait(tt.timeout)

			if !tt.wantErr {
				g.Expect(err).ToNot(HaveOccurred())

				return
			}

			g.Expect(err).To(HaveOccurred())

			var structured *clierrors.StructuredError
			g.Expect(errors.As(err, &structured)).To(BeTrue())
			g.Expect(structured).To(HaveField("Code", tt.wantCode))
		})
	}
}

func TestHasWaitMode(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	g.Expect((&cmd.WaitOptions{}).HasWaitMode()).To(BeFalse())
	g.Expect((&cmd.WaitOptions{WaitFor: "healthy"}).HasWaitMode()).To(BeTrue())
}

const testPollInterval = 1 * time.Second

func TestRunWait(t *testing.T) {
	t.Parallel()

	t.Run("condition met immediately", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		opts := cmd.WaitOptions{WaitFor: "healthy", PollInterval: testPollInterval}
		err := opts.RunWait(t.Context(), 5*time.Second, func(_ context.Context) (bool, error) {
			return true, nil
		})
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("condition met after polls", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		var calls atomic.Int32
		opts := cmd.WaitOptions{WaitFor: "healthy", PollInterval: testPollInterval}

		err := opts.RunWait(t.Context(), 5*time.Second, func(_ context.Context) (bool, error) {
			return calls.Add(1) >= 3, nil
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(calls.Load()).To(BeNumerically(">=", 3))
	})

	t.Run("timeout returns structured WAIT_TIMEOUT error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		opts := cmd.WaitOptions{WaitFor: "healthy", PollInterval: testPollInterval}
		err := opts.RunWait(t.Context(), 50*time.Millisecond, func(_ context.Context) (bool, error) {
			return false, nil
		})
		g.Expect(err).To(HaveOccurred())

		var structured *clierrors.StructuredError
		g.Expect(errors.As(err, &structured)).To(BeTrue())
		g.Expect(structured).To(HaveField("Code", "WAIT_TIMEOUT"))
		g.Expect(structured).To(HaveField("Category", clierrors.CategoryTimeout))
	})

	t.Run("condition error stops polling", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		condErr := errors.New("fatal")
		opts := cmd.WaitOptions{WaitFor: "healthy", PollInterval: testPollInterval}

		err := opts.RunWait(t.Context(), 5*time.Second, func(_ context.Context) (bool, error) {
			return false, condErr
		})
		g.Expect(err).To(HaveOccurred())
		g.Expect(err).To(MatchError(ContainSubstring("fatal")))
	})

	t.Run("timeout zero polls until condition", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		var calls atomic.Int32
		opts := cmd.WaitOptions{WaitFor: "healthy", PollInterval: testPollInterval}

		err := opts.RunWait(t.Context(), 0, func(_ context.Context) (bool, error) {
			return calls.Add(1) >= 5, nil
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(calls.Load()).To(BeNumerically(">=", 5))
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		opts := cmd.WaitOptions{WaitFor: "healthy", PollInterval: testPollInterval}
		err := opts.RunWait(ctx, 0, func(_ context.Context) (bool, error) {
			return false, nil
		})
		g.Expect(err).To(HaveOccurred())
	})
}
