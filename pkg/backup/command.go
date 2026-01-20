package backup

//nolint:gci // Blank import required for resolver registration - DO NOT REMOVE
import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/backup/dependencies"
	// Register notebook resolver via init().
	_ "github.com/lburgazzoli/odh-cli/pkg/backup/dependencies/notebooks"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube"
)

const (
	// Directory permissions for backup output directories.
	dirPermissions = 0o755
	// File permissions for backup YAML files.
	filePermissions = 0o644
)

// Command handles the backup operation.
type Command struct {
	*SharedOptions

	OutputDir   string
	StripFields []string
	Includes    []string
	Excludes    []string

	depRegistry *dependencies.Registry
}

// NewCommand creates a new backup Command.
func NewCommand(streams genericiooptions.IOStreams) *Command {
	return &Command{
		SharedOptions: NewSharedOptions(streams),
	}
}

// AddFlags adds flags to the command.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.OutputDir, "output-dir", "", "Output directory for backups (if not specified, dumps to stdout)")
	fs.StringArrayVar(&c.StripFields, "strip", nil, "Field paths to strip (repeatable, e.g., --strip .status)")
	fs.StringArrayVar(&c.Includes, "includes", nil, "Workload types to include (repeatable, e.g., --includes notebooks.kubeflow.org)")
	fs.StringArrayVar(&c.Excludes, "exclude", nil, "Workload types to exclude (repeatable)")
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, "Enable verbose output")
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, "Timeout for backup operation")
}

// Complete populates derived values and performs setup.
func (c *Command) Complete() error {
	if err := c.SharedOptions.Complete(); err != nil {
		return err
	}

	if len(c.Includes) == 0 {
		c.Includes = DefaultWorkloadTypes
	}

	c.StripFields = append(DefaultStripFields, c.StripFields...)

	c.depRegistry = dependencies.NewRegistry()

	return nil
}

// Validate checks that all options are valid.
func (c *Command) Validate() error {
	if err := c.SharedOptions.Validate(); err != nil {
		return err
	}

	return nil
}

// Run executes the backup command.
func (c *Command) Run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	if c.OutputDir != "" {
		if err := os.MkdirAll(c.OutputDir, dirPermissions); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}

	gvrsToBackup := c.resolveWorkloadGVRs()

	if c.Verbose {
		c.IO.Errorf("Backing up %d workload types...\n", len(gvrsToBackup))
	}

	for _, gvr := range gvrsToBackup {
		if err := c.backupWorkloadType(ctx, gvr); err != nil {
			c.IO.Errorf("Warning: Failed to backup %s: %v\n", gvr.Resource, err)
		}
	}

	if c.OutputDir == "" && c.Verbose {
		c.IO.Errorf("Backup complete (stdout)\n")
	} else {
		c.IO.Errorf("Backup complete: %s\n", c.OutputDir)
	}

	return nil
}

// resolveWorkloadGVRs converts include/exclude strings to GVRs.
func (c *Command) resolveWorkloadGVRs() []schema.GroupVersionResource {
	//nolint:prealloc // Size unknown until references are extracted
	var includeGVRs []schema.GroupVersionResource
	for _, include := range c.Includes {
		gvr := parseGVRString(include)
		includeGVRs = append(includeGVRs, gvr)
	}

	excludeSet := make(map[string]bool)
	for _, exclude := range c.Excludes {
		gvr := parseGVRString(exclude)
		excludeSet[gvr.GroupResource().String()] = true
	}

	var result []schema.GroupVersionResource
	for _, gvr := range includeGVRs {
		if !excludeSet[gvr.GroupResource().String()] {
			result = append(result, gvr)
		}
	}

	return result
}

// backupWorkloadType backs up all instances of a workload type.
func (c *Command) backupWorkloadType(ctx context.Context, gvr schema.GroupVersionResource) error {
	instances, err := c.Client.ListResources(ctx, gvr)
	if err != nil {
		return fmt.Errorf("listing resources: %w", err)
	}

	if c.Verbose {
		c.IO.Errorf("  Found %d instances of %s\n", len(instances), gvr.Resource)
	}

	for i := range instances {
		if err := c.backupWorkloadInstance(ctx, gvr, &instances[i]); err != nil {
			c.IO.Errorf("    Warning: Failed to backup %s/%s: %v\n",
				instances[i].GetNamespace(), instances[i].GetName(), err)
		}
	}

	return nil
}

// backupWorkloadInstance backs up a single workload and its dependencies.
func (c *Command) backupWorkloadInstance(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) error {
	namespace := obj.GetNamespace()
	name := obj.GetName()

	if c.Verbose {
		c.IO.Errorf("    Backing up %s/%s...\n", namespace, name)
	}

	if err := c.writeResource(gvr, obj); err != nil {
		return fmt.Errorf("writing workload: %w", err)
	}

	resolver, err := c.depRegistry.GetResolver(gvr)
	if err != nil {
		if c.Verbose {
			c.IO.Errorf("      No dependency resolver for %s, skipping dependencies\n", gvr.Resource)
		}

		return nil
	}

	deps, err := resolver.Resolve(ctx, c.Client, obj)
	if err != nil {
		c.IO.Errorf("      Warning: Failed to discover dependencies: %v\n", err)

		return nil
	}

	if c.Verbose {
		c.IO.Errorf("      Found %d dependencies\n", len(deps))
	}

	for _, dep := range deps {
		if err := c.writeResource(dep.GVR, dep.Resource); err != nil {
			c.IO.Errorf("        Warning: Failed to write dependency %s/%s: %v\n",
				dep.Resource.GetNamespace(), dep.Resource.GetName(), err)
		}
	}

	return nil
}

// writeResource strips fields and writes a resource to the output directory or stdout.
func (c *Command) writeResource(
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) error {
	stripped, err := kube.StripFields(obj, c.StripFields)
	if err != nil {
		return fmt.Errorf("stripping fields: %w", err)
	}

	if c.OutputDir == "" {
		return WriteResourceToStdout(c.IO.Out(), gvr, stripped)
	}

	return WriteResourceToFile(c.OutputDir, gvr, stripped)
}
