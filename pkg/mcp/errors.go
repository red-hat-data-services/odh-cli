package mcp

import (
	"encoding/json"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"

	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

func mapErrorToResult(err error) *mcp.CallToolResult {
	if err == nil {
		return nil
	}

	if errors.Is(err, clierrors.ErrAlreadyHandled) {
		err = unwrapAlreadyHandled(err)
	}

	result := *clierrors.Classify(err)

	if result.ExitCode == 0 {
		result.ExitCode = int(clierrors.ExitCodeFromCategory(result.Category))
	}

	errorJSON, marshalErr := json.Marshal(clierrors.ErrorEnvelope{Error: &result})
	if marshalErr != nil {
		return mcp.NewToolResultError(err.Error())
	}

	return mcp.NewToolResultError(string(errorJSON))
}

func unwrapAlreadyHandled(err error) error {
	var structured *clierrors.StructuredError
	if errors.As(err, &structured) {
		return structured
	}

	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, inner := range multi.Unwrap() {
			if !errors.Is(inner, clierrors.ErrAlreadyHandled) {
				return inner
			}
		}
	}

	return err
}
