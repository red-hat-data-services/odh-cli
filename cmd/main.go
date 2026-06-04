package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/cmd/completion"
	"github.com/opendatahub-io/odh-cli/cmd/components"
	"github.com/opendatahub-io/odh-cli/cmd/deps"
	"github.com/opendatahub-io/odh-cli/cmd/events"
	"github.com/opendatahub-io/odh-cli/cmd/get"
	"github.com/opendatahub-io/odh-cli/cmd/lint"
	"github.com/opendatahub-io/odh-cli/cmd/logs"
	"github.com/opendatahub-io/odh-cli/cmd/migrate"
	"github.com/opendatahub-io/odh-cli/cmd/status"
	"github.com/opendatahub-io/odh-cli/cmd/version"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

func main() {
	flags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()

	cmd := &cobra.Command{
		Use:   "kubectl-odh",
		Short: "kubectl plugin for ODH/RHOAI",
	}

	// Add kubectl-style flags to root command (inherited by subcommands).
	// This exposes standard authentication flags: --server, --username, --password,
	// --token, --kubeconfig, --context, --cluster, --certificate-authority,
	// --client-certificate, --client-key, --insecure-skip-tls-verify, etc.
	flags.AddFlags(cmd.PersistentFlags())

	version.AddCommand(cmd, flags)
	lint.AddCommand(cmd, flags)
	get.AddCommand(cmd, flags)
	deps.AddCommand(cmd, flags)
	components.AddCommand(cmd, flags)
	status.AddCommand(cmd, flags)
	logs.AddCommand(cmd, flags)
	completion.AddCommand(cmd, flags)
	migrate.AddCommand(cmd, flags)
	events.AddCommand(cmd, flags)

	if err := cmd.Execute(); err != nil {
		exitCode := int(clierrors.ExitCodeFromError(err))

		if !errors.Is(err, clierrors.ErrAlreadyHandled) {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
		}

		os.Exit(exitCode)
	}
}
