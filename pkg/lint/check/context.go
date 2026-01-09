package check

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrCheckTimeout is returned when a check exceeds its timeout.
	ErrCheckTimeout = errors.New("check execution timed out")

	// ErrCheckCanceled is returned when a check's context is canceled.
	ErrCheckCanceled = errors.New("check execution canceled")
)

// CheckContextError returns an error if the context is done.
// Returns nil if the context is still valid.
//
// This function should be called before long-running operations within checks
// to respect command-level timeouts and allow graceful cancellation.
//
// Usage in check implementations:
//
//	if err := check.CheckContextError(ctx); err != nil {
//	    return nil, err
//	}
//
// The function uses a non-blocking select to avoid delays when the context
// is still valid. Overhead is minimal (~1-2 nanoseconds per call).
func CheckContextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		err := ctx.Err()
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrCheckTimeout
		}
		if errors.Is(err, context.Canceled) {
			return ErrCheckCanceled
		}

		return fmt.Errorf("context error: %w", err)
	default:
		return nil
	}
}
