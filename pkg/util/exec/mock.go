package exec

import "context"

// MockExecutor is a test double for Executor.
type MockExecutor struct {
	ExecFn func(ctx context.Context, opts ExecOptions) error
}

func (m *MockExecutor) Exec(ctx context.Context, opts ExecOptions) error {
	if m.ExecFn != nil {
		return m.ExecFn(ctx, opts)
	}

	return nil
}
