package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"

	. "github.com/onsi/gomega"
)

const watchTestPollInterval = 1 * time.Second

func TestValidateWatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		watch    bool
		wait     bool
		format   string
		wantErr  bool
		wantCode string
	}{
		{
			name:   "watch off passes",
			watch:  false,
			format: "table",
		},
		{
			name:   "watch with json passes",
			watch:  true,
			format: "json",
		},
		{
			name:   "watch with yaml passes",
			watch:  true,
			format: "yaml",
		},
		{
			name:     "watch with table rejected",
			watch:    true,
			format:   "table",
			wantErr:  true,
			wantCode: "WATCH_OUTPUT_FORMAT",
		},
		{
			name:     "watch with wait-for rejected",
			watch:    true,
			wait:     true,
			format:   "json",
			wantErr:  true,
			wantCode: "WATCH_WAIT_EXCLUSIVE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			opts := cmd.WatchOptions{Watch: tt.watch}
			err := opts.ValidateWatch(tt.wait, tt.format, []string{"json", "yaml"}, 30*time.Second, cmd.DefaultPollInterval)

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

func TestRunWatch(t *testing.T) {
	t.Parallel()

	t.Run("emits initial data immediately", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())
		var emitted atomic.Int32

		opts := cmd.WatchOptions{Watch: true}
		err := opts.RunWatch(ctx, cmd.WatchRunConfig{
			PollInterval: watchTestPollInterval,
			Poller: func(_ context.Context) (any, []byte, error) {
				return "data", []byte("snap"), nil
			},
			Emitter: func(data any) error {
				emitted.Add(1)
				cancel()

				return nil
			},
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(emitted.Load()).To(Equal(int32(1)))
	})

	t.Run("suppresses duplicate snapshots", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())
		var emitted atomic.Int32
		var polls atomic.Int32

		opts := cmd.WatchOptions{Watch: true}
		err := opts.RunWatch(ctx, cmd.WatchRunConfig{
			PollInterval: watchTestPollInterval,
			Poller: func(_ context.Context) (any, []byte, error) {
				if polls.Add(1) >= 3 {
					cancel()
				}

				return "data", []byte("same-snap"), nil
			},
			Emitter: func(data any) error {
				emitted.Add(1)

				return nil
			},
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(emitted.Load()).To(Equal(int32(1)))
	})

	t.Run("emits on state change", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())
		var polls atomic.Int32
		var emitted atomic.Int32

		opts := cmd.WatchOptions{Watch: true}
		err := opts.RunWatch(ctx, cmd.WatchRunConfig{
			PollInterval: watchTestPollInterval,
			Poller: func(_ context.Context) (any, []byte, error) {
				n := polls.Add(1)
				snap := []byte("snap-" + string('0'+n))

				if n >= 3 {
					cancel()
				}

				return "data", snap, nil
			},
			Emitter: func(data any) error {
				emitted.Add(1)

				return nil
			},
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(emitted.Load()).To(BeNumerically(">=", 3))
	})

	t.Run("transient error retried with warning", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())
		var polls atomic.Int32
		var stderr bytes.Buffer

		opts := cmd.WatchOptions{Watch: true}
		err := opts.RunWatch(ctx, cmd.WatchRunConfig{
			PollInterval: watchTestPollInterval,
			ErrOut:       &stderr,
			Poller: func(_ context.Context) (any, []byte, error) {
				n := polls.Add(1)
				if n == 2 {
					return nil, nil, errors.New("transient")
				}
				if n >= 3 {
					cancel()
				}

				return "data", []byte("snap"), nil
			},
			Emitter: func(data any) error {
				return nil
			},
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(stderr.String()).To(ContainSubstring("Warning: poll failed"))
	})

	t.Run("cancelled context exits cleanly", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		opts := cmd.WatchOptions{Watch: true}
		err := opts.RunWatch(ctx, cmd.WatchRunConfig{
			PollInterval: watchTestPollInterval,
			Poller: func(_ context.Context) (any, []byte, error) {
				return "data", []byte("snap"), nil
			},
			Emitter: func(data any) error {
				return nil
			},
		})
		g.Expect(err).ToNot(HaveOccurred())
	})
}
