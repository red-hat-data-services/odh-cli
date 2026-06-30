//nolint:testpackage // Tests internal mapErrorToResult function
package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"

	. "github.com/onsi/gomega"
)

func extractErrorJSON(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	g := NewWithT(t)

	g.Expect(result.Content).To(HaveLen(1))

	textContent, ok := result.Content[0].(mcp.TextContent)
	g.Expect(ok).To(BeTrue(), "expected TextContent, got %T", result.Content[0])

	var envelope map[string]map[string]any
	err := json.Unmarshal([]byte(textContent.Text), &envelope)
	g.Expect(err).ToNot(HaveOccurred())

	return envelope["error"]
}

func TestMapErrorToResult(t *testing.T) {
	t.Run("should return nil for nil error", func(t *testing.T) {
		g := NewWithT(t)

		result := mapErrorToResult(nil)

		g.Expect(result).To(BeNil())
	})

	t.Run("should return error result for plain error", func(t *testing.T) {
		g := NewWithT(t)

		result := mapErrorToResult(errors.New("something broke"))

		g.Expect(result).ToNot(BeNil())
		g.Expect(result.IsError).To(BeTrue())

		errData := extractErrorJSON(t, result)
		g.Expect(errData["code"]).To(Equal("INTERNAL"))
		g.Expect(errData["category"]).To(Equal("internal"))
		g.Expect(errData["retriable"]).To(BeFalse())
		g.Expect(errData["suggestion"]).ToNot(BeEmpty())
	})

	t.Run("should unwrap ErrAlreadyHandled", func(t *testing.T) {
		g := NewWithT(t)

		inner := clierrors.NewValidationError("BAD_INPUT", "bad input", "fix it")
		wrapped := clierrors.NewAlreadyHandledError(inner)

		result := mapErrorToResult(wrapped)

		g.Expect(result.IsError).To(BeTrue())

		errData := extractErrorJSON(t, result)
		g.Expect(errData["code"]).To(Equal("BAD_INPUT"))
		g.Expect(errData["category"]).To(Equal("validation"))
	})

	t.Run("should pass through already classified StructuredError", func(t *testing.T) {
		g := NewWithT(t)

		structured := &clierrors.StructuredError{
			Code:       "CUSTOM_CODE",
			Message:    "custom message",
			Category:   clierrors.CategoryTimeout,
			Retriable:  true,
			Suggestion: "try again",
		}

		result := mapErrorToResult(structured)

		g.Expect(result.IsError).To(BeTrue())

		errData := extractErrorJSON(t, result)
		g.Expect(errData["code"]).To(Equal("CUSTOM_CODE"))
		g.Expect(errData["message"]).To(Equal("custom message"))
		g.Expect(errData["category"]).To(Equal("timeout"))
		g.Expect(errData["retriable"]).To(BeTrue())
		g.Expect(errData["suggestion"]).To(Equal("try again"))
	})

	t.Run("should classify wrapped errors", func(t *testing.T) {
		g := NewWithT(t)

		inner := clierrors.NewValidationError("VERSION_UNAVAILABLE", "not available", "use --refresh")
		wrapped := fmt.Errorf("get manifest: %w", inner)

		result := mapErrorToResult(wrapped)

		errData := extractErrorJSON(t, result)
		g.Expect(errData["code"]).To(Equal("VERSION_UNAVAILABLE"))
		g.Expect(errData["category"]).To(Equal("validation"))
	})

	t.Run("should include message in error JSON", func(t *testing.T) {
		g := NewWithT(t)

		result := mapErrorToResult(errors.New("detailed failure reason"))

		errData := extractErrorJSON(t, result)
		g.Expect(errData["message"]).To(ContainSubstring("detailed failure reason"))
	})

	t.Run("should strip sentinel text from ErrAlreadyHandled wrapping plain error", func(t *testing.T) {
		g := NewWithT(t)

		inner := errors.New("connection refused")
		wrapped := clierrors.NewAlreadyHandledError(inner)

		result := mapErrorToResult(wrapped)

		g.Expect(result.IsError).To(BeTrue())

		errData := extractErrorJSON(t, result)
		g.Expect(errData["message"]).To(ContainSubstring("connection refused"))
		g.Expect(errData["message"]).ToNot(ContainSubstring("already rendered"))
	})

	t.Run("should backfill exitCode from category for validation errors", func(t *testing.T) {
		g := NewWithT(t)

		validationErr := clierrors.NewValidationError("BAD_INPUT", "bad input", "fix it")

		result := mapErrorToResult(validationErr)

		errData := extractErrorJSON(t, result)
		g.Expect(errData["exitCode"]).ToNot(BeZero(), "exitCode should be backfilled from category, not 0")
	})
}
