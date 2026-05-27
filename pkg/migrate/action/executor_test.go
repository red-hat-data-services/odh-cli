package action_test

import (
	"context"
	"errors"
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"

	. "github.com/onsi/gomega"
)

func TestExecutor_ExecuteAll(t *testing.T) {
	t.Run("should return empty for empty registry", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()
		executor := action.NewExecutor(registry)

		results := executor.ExecuteAll(context.Background(), action.Target{})
		g.Expect(results).To(BeEmpty())
	})

	t.Run("should skip actions where CanApply returns false", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{
			id:       "skip.action",
			canApply: false,
			runTask:  &mockTask{result: result.New("migration", "skip", "Skip", "")},
		})

		executor := action.NewExecutor(registry)
		results := executor.ExecuteAll(context.Background(), action.Target{})
		g.Expect(results).To(BeEmpty())
	})

	t.Run("should execute applicable actions", func(t *testing.T) {
		g := NewWithT(t)
		actionResult := result.New("migration", "test", "Test", "")
		actionResult.Status.Completed = true

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{
			id:       "test.action",
			canApply: true,
			runTask:  &mockTask{result: actionResult},
		})

		executor := action.NewExecutor(registry)
		results := executor.ExecuteAll(context.Background(), action.Target{})
		g.Expect(results).To(HaveLen(1))
		g.Expect(results[0].Error).ToNot(HaveOccurred())
		g.Expect(results[0].Result.Status.Completed).To(BeTrue())
	})

	t.Run("should handle action with nil run task", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{
			id:       "nil.task",
			canApply: true,
			runTask:  nil,
		})

		executor := action.NewExecutor(registry)
		results := executor.ExecuteAll(context.Background(), action.Target{})
		g.Expect(results).To(HaveLen(1))
		g.Expect(results[0].Error).To(HaveOccurred())
		g.Expect(results[0].Error.Error()).To(ContainSubstring("no run task"))
	})

	t.Run("should handle task execution error", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{
			id:       "error.action",
			canApply: true,
			runTask:  &mockTask{err: errors.New("execution failed")},
		})

		executor := action.NewExecutor(registry)
		results := executor.ExecuteAll(context.Background(), action.Target{})
		g.Expect(results).To(HaveLen(1))
		g.Expect(results[0].Error).To(HaveOccurred())
		g.Expect(results[0].Result.Status.Error).To(ContainSubstring("execution failed"))
	})
}

func TestExecutor_ExecuteSelective(t *testing.T) {
	t.Run("should filter by pattern", func(t *testing.T) {
		g := NewWithT(t)
		actionResult := result.New("migration", "test", "Test", "")
		actionResult.Status.Completed = true

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{
			id:       "kueue.migrate",
			canApply: true,
			runTask:  &mockTask{result: actionResult},
		})
		registry.MustRegister(&mockAction{
			id:       "other.migrate",
			canApply: true,
			runTask:  &mockTask{result: actionResult},
		})

		executor := action.NewExecutor(registry)
		results, err := executor.ExecuteSelective(context.Background(), action.Target{}, "kueue.*", "", "")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(results).To(HaveLen(1))
		g.Expect(results[0].Action.ID()).To(Equal("kueue.migrate"))
	})

	t.Run("should filter by phase", func(t *testing.T) {
		g := NewWithT(t)
		actionResult := result.New("migration", "test", "Test", "")
		actionResult.Status.Completed = true

		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{
			id:       "pre.action",
			phase:    action.PhasePreUpgrade,
			canApply: true,
			runTask:  &mockTask{result: actionResult},
		})
		registry.MustRegister(&mockAction{
			id:       "post.action",
			phase:    action.PhasePostUpgrade,
			canApply: true,
			runTask:  &mockTask{result: actionResult},
		})

		executor := action.NewExecutor(registry)
		results, err := executor.ExecuteSelective(context.Background(), action.Target{}, "*", "", action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(results).To(HaveLen(1))
		g.Expect(results[0].Action.ID()).To(Equal("pre.action"))
	})

	t.Run("should return error for invalid pattern", func(t *testing.T) {
		g := NewWithT(t)
		registry := action.NewActionRegistry()
		registry.MustRegister(&mockAction{id: "test.action", canApply: true})
		executor := action.NewExecutor(registry)

		_, err := executor.ExecuteSelective(context.Background(), action.Target{}, "[invalid", "", "")
		g.Expect(err).To(HaveOccurred())
	})
}
