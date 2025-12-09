package lint

import (
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

// CommandOptions contains configuration for creating a Command using struct-based initialization.
// This is the preferred pattern for simple command construction.
//
// Example:
//
//	cmd := lint.NewCommandWithOptions(lint.CommandOptions{
//	    Streams:       streams,
//	    TargetVersion: "3.0",
//	})
type CommandOptions struct {
	// Streams provides access to stdin, stdout, stderr
	Streams genericiooptions.IOStreams

	// TargetVersion is the optional target version for upgrade assessment
	TargetVersion string

	// Shared allows passing a pre-configured SharedOptions (advanced use case)
	Shared *SharedOptions
}

// CommandOption is a functional option for configuring a Command.
// This pattern is useful for complex initialization scenarios.
//
// Example:
//
//	cmd := lint.NewCommandWithFunctionalOptions(
//	    lint.WithStreams(streams),
//	    lint.WithTargetVersion("3.0"),
//	)
type CommandOption func(*Command)

// WithStreams returns a CommandOption that sets the IO streams.
func WithStreams(streams genericiooptions.IOStreams) CommandOption {
	return func(c *Command) {
		if c.SharedOptions == nil {
			c.SharedOptions = NewSharedOptions(streams)
		} else {
			// Update existing SharedOptions streams
			c.IO = iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut)
		}
	}
}

// WithTargetVersion returns a CommandOption that sets the target version.
func WithTargetVersion(version string) CommandOption {
	return func(c *Command) {
		c.TargetVersion = version
	}
}

// WithShared returns a CommandOption that sets the SharedOptions.
// This is an advanced option for cases where SharedOptions needs custom configuration.
func WithShared(shared *SharedOptions) CommandOption {
	return func(c *Command) {
		c.SharedOptions = shared
	}
}

// NewCommandWithOptions creates a new Command using struct-based initialization.
// This is the preferred pattern for simple command construction.
func NewCommandWithOptions(opts CommandOptions) *Command {
	var shared *SharedOptions
	if opts.Shared != nil {
		shared = opts.Shared
	} else {
		shared = NewSharedOptions(opts.Streams)
	}

	return &Command{
		SharedOptions: shared,
		TargetVersion: opts.TargetVersion,
	}
}

// NewCommandWithFunctionalOptions creates a new Command using functional options.
// This pattern is useful for complex initialization scenarios.
func NewCommandWithFunctionalOptions(options ...CommandOption) *Command {
	// Initialize with default empty streams
	cmd := &Command{
		SharedOptions: NewSharedOptions(genericiooptions.IOStreams{}),
	}

	// Apply functional options
	for _, opt := range options {
		opt(cmd)
	}

	return cmd
}
