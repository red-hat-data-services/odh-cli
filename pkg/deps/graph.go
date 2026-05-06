package deps

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	utilserrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

const minHeaderWidth = 40

// Verify GraphCommand implements cmd.Command interface at compile time.
var _ cmd.Command = (*GraphCommand)(nil)

// GraphCommand holds the deps graph command configuration.
type GraphCommand struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags

	Refresh         bool
	Version         string
	manifestVersion string
}

// NewGraphCommand creates a new GraphCommand with defaults.
func NewGraphCommand(streams genericiooptions.IOStreams, configFlags *genericclioptions.ConfigFlags) *GraphCommand {
	return &GraphCommand{
		IO:          iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags: configFlags,
	}
}

// AddFlags registers command-specific flags with the provided FlagSet.
func (c *GraphCommand) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.Refresh, "refresh", false, "Fetch latest manifest from odh-gitops")
	fs.StringVar(&c.Version, "version", "", "ODH/RHOAI version to show dependencies for")
}

// Complete prepares the command for execution.
func (c *GraphCommand) Complete() error {
	return nil
}

// Validate checks the command options.
func (c *GraphCommand) Validate() error {
	// Skip version validation if refreshing (will fetch from remote)
	if c.Refresh {
		return nil
	}

	// Validate version against embedded manifest
	manifestVer, err := ManifestVersion()
	if err != nil {
		return fmt.Errorf("failed to determine manifest version: %w", err)
	}

	c.manifestVersion = manifestVer

	if c.Version != "" && !majorMinorMatch(c.Version, c.manifestVersion) {
		return utilserrors.NewValidationError(
			"VERSION_UNAVAILABLE",
			fmt.Sprintf("dependency graph for version %q is not available; only version %s is supported", c.Version, c.manifestVersion),
			"Use --refresh to fetch the latest manifest, or omit --version to use the embedded version",
		)
	}

	return nil
}

// Run executes the graph command.
func (c *GraphCommand) Run(ctx context.Context) error {
	result, err := GetManifest(ctx, c.Refresh)
	if err != nil {
		return fmt.Errorf("get manifest: %w", err)
	}

	// Update manifest version from result
	if result.Version != "" {
		c.manifestVersion = result.Version
	}

	// Validate version if specified (for refresh case, validation happens here)
	if c.Version != "" && !majorMinorMatch(c.Version, c.manifestVersion) {
		suggestion := "Use --refresh to fetch the latest manifest, or omit --version to use the embedded version"
		if c.Refresh {
			suggestion = "Omit --version to use the fetched manifest version"
		}

		return utilserrors.NewValidationError(
			"VERSION_UNAVAILABLE",
			fmt.Sprintf("dependency graph for version %q is not available; only version %s is supported", c.Version, c.manifestVersion),
			suggestion,
		)
	}

	manifest := result.Manifest
	w := c.IO.Out()

	header := "Dependency Graph for ODH/RHOAI " + c.manifestVersion
	headerWidth := max(len(header), minHeaderWidth)

	_, _ = fmt.Fprintln(w, header)
	_, _ = fmt.Fprintln(w, strings.Repeat("=", headerWidth))
	_, _ = fmt.Fprintln(w)

	deps := manifest.GetDependencies()
	depMap := buildDepMap(manifest)

	for _, dep := range deps {
		c.printDepNode(dep, depMap)
	}

	return nil
}

// buildDepMap creates a map of dependency name to its transitive dependencies.
// It computes the full transitive closure, so A -> B -> C will show both B and C for A.
func buildDepMap(manifest *Manifest) map[string][]string {
	depMap := make(map[string][]string)

	for name := range manifest.Dependencies {
		visited := make(map[string]bool)
		collectTransitiveDeps(manifest, name, visited)

		// Remove self from visited set
		delete(visited, name)

		transDeps := make([]string, 0, len(visited))
		for depName := range visited {
			transDeps = append(transDeps, toDisplayName(depName))
		}

		sort.Strings(transDeps)
		depMap[name] = transDeps
	}

	return depMap
}

// collectTransitiveDeps recursively collects all transitive dependencies.
func collectTransitiveDeps(manifest *Manifest, name string, visited map[string]bool) {
	if visited[name] {
		return
	}

	visited[name] = true

	dep, ok := manifest.Dependencies[name]
	if !ok {
		return
	}

	for transName, val := range dep.Dependencies {
		if isEnabled(val) {
			collectTransitiveDeps(manifest, transName, visited)
		}
	}
}

func (c *GraphCommand) printDepNode(dep DependencyInfo, depMap map[string][]string) {
	w := c.IO.Out()

	_, _ = fmt.Fprintf(w, "%s\n", dep.DisplayName)

	requiredBy := "(none)"
	if len(dep.RequiredBy) > 0 {
		requiredBy = strings.Join(dep.RequiredBy, ", ")
	}

	dependsOn := "(none)"
	if transDeps, ok := depMap[dep.Name]; ok && len(transDeps) > 0 {
		dependsOn = strings.Join(transDeps, ", ")
	}

	_, _ = fmt.Fprintf(w, "├── Required by: %s\n", requiredBy)
	_, _ = fmt.Fprintf(w, "└── Depends on: %s\n", dependsOn)
	_, _ = fmt.Fprintln(w)
}
