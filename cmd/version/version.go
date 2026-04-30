package version

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/internal/version"
)

const (
	cmdName  = "version"
	cmdShort = "Show version information"
)

// AddCommand adds the version subcommand to the root command.
func AddCommand(root *cobra.Command, _ *genericclioptions.ConfigFlags) {
	var (
		outputFormat string
		verbose      bool
		quiet        bool
	)

	cmd := &cobra.Command{
		Use:          cmdName,
		Short:        cmdShort,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if verbose && quiet {
				return errors.New("--verbose and --quiet are mutually exclusive")
			}

			// Determine output writer (suppress if quiet)
			out := cmd.OutOrStdout()
			if quiet {
				out = io.Discard
			}

			switch outputFormat {
			case "json":
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")

				data := map[string]string{
					"version": version.GetVersion(),
					"commit":  version.GetCommit(),
					"date":    version.GetDate(),
				}

				if verbose {
					data["goVersion"] = runtime.Version()
					data["platform"] = runtime.GOOS + "/" + runtime.GOARCH
				}

				err := encoder.Encode(data)
				if err != nil {
					return fmt.Errorf("failed to encode version information as JSON: %w", err)
				}

				return nil
			default:
				if verbose {
					_, err := fmt.Fprintf(
						out,
						"kubectl-odh version %s\n  Commit:     %s\n  Built:      %s\n  Go version: %s\n  Platform:   %s\n",
						version.GetVersion(),
						version.GetCommit(),
						version.GetDate(),
						runtime.Version(),
						runtime.GOOS+"/"+runtime.GOARCH,
					)

					if err != nil {
						return fmt.Errorf("failed to write version information: %w", err)
					}

					return nil
				}

				_, err := fmt.Fprintf(
					out,
					"kubectl-odh version %s (commit: %s, built: %s)\n",
					version.GetVersion(),
					version.GetCommit(),
					version.GetDate(),
				)

				if err != nil {
					return fmt.Errorf("failed to write version information: %w", err)
				}

				return nil
			}
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text|json)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all output")

	root.AddCommand(cmd)
}
