package deps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	utilserrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

// Verify InstallCommand implements cmd.Command interface at compile time.
var _ cmd.Command = (*InstallCommand)(nil)

const (
	defaultTimeout       = 5 * time.Minute
	pollInterval         = 5 * time.Second
	defaultCatalogSource = "redhat-operators"
	sourceNamespace      = "openshift-marketplace"

	// csvNamespaceListEveryNPolls limits full-namespace CSV List calls during wait:
	// only every Nth poll (subscription Get still runs each poll).
	csvNamespaceListEveryNPolls = 3

	depInstallCheckConcurrency = 8

	// Label for resources created by odh-cli.
	labelManagedBy      = "app.kubernetes.io/managed-by"
	labelManagedByValue = "odh-cli"

	// Error messages.
	msgCreateClientInstall = "create kubernetes client: %w"
	msgOLMNotAvailableInst = "OLM (Operator Lifecycle Manager) is not available; cannot install dependencies"
	msgUnknownDependency   = "unknown dependency %q; run 'kubectl odh deps' to see available dependencies"

	// Progress messages.
	msgInstalling            = "Installing %s...\n"
	msgCreatingNamespace     = "  Creating namespace %s\n"
	msgEnsuringOperatorGroup = "  Ensuring OperatorGroup...\n"
	msgCreatedOperatorGroup  = "  Created OperatorGroup\n"
	msgOperatorGroupExists   = "  OperatorGroup already exists\n"
	msgCreatingSubscription  = "  Creating Subscription"
	msgWaitingForCSV         = "  Waiting for CSV... (%s)\n"
	msgWaitingForCSVPhase    = "  Waiting for CSV %s... (%s)\n"
	msgAllInstalled          = "All dependencies are already installed."
	msgNoDepsToInstall       = "No dependencies to install."

	// Success/failure messages.
	msgSuccessInstall = "✓ %s %s installed\n"
	msgFailInstall    = "✗ %s failed: %v\n"
)

// InstallResult tracks the outcome of a single dependency installation.
type InstallResult struct {
	Name        string
	DisplayName string
	Status      InstallStatus
	Version     string
	Error       error
}

// InstallStatus represents the outcome of an installation attempt.
type InstallStatus string

const (
	InstallStatusInstalled InstallStatus = "installed"
	InstallStatusFailed    InstallStatus = "failed"
)

// syncWriter wraps an io.Writer with a mutex for concurrent-safe writes.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (sw *syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	n, err := sw.w.Write(p)
	if err != nil {
		return n, fmt.Errorf("write: %w", err)
	}

	return n, nil
}

// InstallCommand holds the deps install command configuration.
type InstallCommand struct {
	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags

	DryRun          bool
	IncludeOptional bool
	Timeout         time.Duration
	TargetDep       string
	Version         string
	Refresh         bool

	client          client.Client
	manifestVersion string
	useColor        bool
}

// NewInstallCommand creates a new InstallCommand with defaults.
func NewInstallCommand(streams genericiooptions.IOStreams, configFlags *genericclioptions.ConfigFlags) *InstallCommand {
	return &InstallCommand{
		IO:          iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags: configFlags,
		Timeout:     defaultTimeout,
	}
}

// AddFlags registers command-specific flags with the provided FlagSet.
func (c *InstallCommand) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.DryRun, "dry-run", false, "Show what would be installed without executing")
	fs.BoolVar(&c.IncludeOptional, "include-optional", false, "Install optional dependencies in addition to required")
	fs.DurationVar(&c.Timeout, "timeout", defaultTimeout, "Timeout for waiting on each operator CSV")
	fs.StringVar(&c.Version, "version", "", "ODH/RHOAI version to install dependencies for")
	fs.BoolVar(&c.Refresh, "refresh", false, "Fetch latest manifest from odh-gitops")
}

// Complete prepares the command for execution.
func (c *InstallCommand) Complete() error {
	c.useColor = shouldUseColor(c.IO.Out())

	if c.DryRun {
		return nil
	}

	cl, err := client.NewClient(c.ConfigFlags)
	if err != nil {
		return fmt.Errorf(msgCreateClientInstall, err)
	}

	c.client = cl

	return nil
}

// Validate checks the command options.
func (c *InstallCommand) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("--timeout must be positive, got %v", c.Timeout)
	}

	if !c.DryRun && !c.client.OLM().Available() {
		return errors.New(msgOLMNotAvailableInst)
	}

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
			fmt.Sprintf("dependencies for version %q are not available; only version %s is supported", c.Version, c.manifestVersion),
			"Use --refresh to fetch the latest manifest, or omit --version to use the embedded version",
		)
	}

	return nil
}

// Run executes the install command.
func (c *InstallCommand) Run(ctx context.Context) error {
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
			fmt.Sprintf("dependencies for version %q are not available; only version %s is supported", c.Version, c.manifestVersion),
			suggestion,
		)
	}

	w := c.IO.Out()
	_, _ = fmt.Fprintf(w, "Installing dependencies for ODH/RHOAI %s\n\n", c.manifestVersion)

	deps := result.Manifest.GetDependencies()

	if c.TargetDep != "" {
		if !c.isValidDependency(deps, c.TargetDep) {
			return fmt.Errorf(msgUnknownDependency, c.TargetDep)
		}
	}

	if c.DryRun {
		return c.runDryRun(ctx, deps)
	}

	return c.runInstall(ctx, deps)
}

func (c *InstallCommand) isValidDependency(deps []DependencyInfo, name string) bool {
	for _, d := range deps {
		if d.Name == name {
			return true
		}
	}

	return false
}

func (c *InstallCommand) runDryRun(_ context.Context, deps []DependencyInfo) error {
	w := c.IO.Out()

	_, _ = fmt.Fprintln(w, "[DRY RUN] The following resources would be created:")
	_, _ = fmt.Fprintln(w)

	toInstall := c.filterDepsForDryRun(deps)
	if len(toInstall) == 0 {
		_, _ = fmt.Fprintln(w, msgNoDepsToInstall)

		return nil
	}

	for _, dep := range toInstall {
		c.printDryRunManifests(w, dep)
	}

	return nil
}

func (c *InstallCommand) printDryRunManifests(w io.Writer, dep DependencyInfo) {
	_, _ = fmt.Fprintf(w, "# %s\n", dep.DisplayName)
	_, _ = fmt.Fprintln(w, "---")

	// Namespace
	_, _ = fmt.Fprintf(w, `apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    %s: %s
---
`, dep.Namespace, labelManagedBy, labelManagedByValue)

	// OperatorGroup
	_, _ = fmt.Fprintf(w, `apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: %s-og
  namespace: %s
  labels:
    %s: %s
spec:
`, dep.Namespace, dep.Namespace, labelManagedBy, labelManagedByValue)

	if len(dep.TargetNamespaces) > 0 {
		_, _ = fmt.Fprintln(w, "  targetNamespaces:")
		for _, ns := range dep.TargetNamespaces {
			_, _ = fmt.Fprintf(w, "    - %s\n", ns)
		}
	}

	_, _ = fmt.Fprintln(w, "---")

	// Subscription
	source := dep.Source
	if source == "" {
		source = defaultCatalogSource
	}

	_, _ = fmt.Fprintf(w, `apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: %s
  namespace: %s
  labels:
    %s: %s
spec:
  channel: %s
  name: %s
  source: %s
  sourceNamespace: %s
  installPlanApproval: Automatic
---
`, dep.Subscription, dep.Namespace, labelManagedBy, labelManagedByValue, dep.Channel, dep.Subscription, source, sourceNamespace)
}

func (c *InstallCommand) runInstall(ctx context.Context, deps []DependencyInfo) error {
	w := c.IO.Out()

	toInstall, err := c.filterDepsToInstall(ctx, deps)
	if err != nil {
		return fmt.Errorf("filter dependencies: %w", err)
	}

	if len(toInstall) == 0 {
		_, _ = fmt.Fprintln(w, msgAllInstalled)

		return nil
	}

	// Phase 1: Create all resources (namespace, operatorgroup, subscription)
	results := make([]InstallResult, len(toInstall))
	pendingCSVs := make([]int, 0, len(toInstall))

	for i, dep := range toInstall {
		results[i] = InstallResult{
			Name:        dep.Name,
			DisplayName: dep.DisplayName,
		}

		if err := c.createDepResources(ctx, dep); err != nil {
			results[i].Status = InstallStatusFailed
			results[i].Error = err
			c.printFailure(w, dep.DisplayName, err)
		} else {
			pendingCSVs = append(pendingCSVs, i)
		}
	}

	// Phase 2: Wait for all CSVs in parallel
	if len(pendingCSVs) > 0 {
		_, _ = fmt.Fprintf(w, "\nWaiting for %d operator(s) to become ready...\n", len(pendingCSVs))
		c.waitForCSVsParallel(ctx, toInstall, results, pendingCSVs)
	}

	c.printSummary(w, results)

	// Collect all errors for full error chain
	var errs []error

	for _, r := range results {
		if r.Status == InstallStatusFailed && r.Error != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.Name, r.Error))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (c *InstallCommand) waitForCSVsParallel(ctx context.Context, deps []DependencyInfo, results []InstallResult, pendingIndices []int) {
	sw := &syncWriter{w: c.IO.Out()}

	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for _, idx := range pendingIndices {
		dep := deps[idx]
		resultIdx := idx

		g.Go(func() error {
			version, err := c.waitForCSV(gctx, sw, dep.Namespace, dep.Subscription)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				results[resultIdx].Status = InstallStatusFailed
				results[resultIdx].Error = err
				c.printFailure(sw, dep.DisplayName, err)
			} else {
				results[resultIdx].Status = InstallStatusInstalled
				results[resultIdx].Version = version
				c.printSuccess(sw, dep.DisplayName, version)
			}

			return nil // Don't fail the group, we track errors in results
		})
	}

	_ = g.Wait()
}

func (c *InstallCommand) filterDepsForDryRun(deps []DependencyInfo) []DependencyInfo {
	var toInstall []DependencyInfo

	for _, dep := range deps {
		if c.TargetDep != "" && dep.Name != c.TargetDep {
			continue
		}

		if !c.shouldInstallDep(dep) {
			continue
		}

		toInstall = append(toInstall, dep)
	}

	return toInstall
}

func (c *InstallCommand) filterDepsToInstall(ctx context.Context, deps []DependencyInfo) ([]DependencyInfo, error) {
	indices := make([]int, 0, len(deps))

	for i, dep := range deps {
		if c.TargetDep != "" && dep.Name != c.TargetDep {
			continue
		}

		if !c.shouldInstallDep(dep) {
			continue
		}

		indices = append(indices, i)
	}

	if len(indices) == 0 {
		return nil, nil
	}

	installed := make([]bool, len(indices))

	g, gctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, depInstallCheckConcurrency)

	for j, depIdx := range indices {
		dep := deps[depIdx]

		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()

			ok, err := c.isAlreadyInstalled(gctx, dep)
			installed[j] = ok

			return err
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("check installed subscriptions: %w", err)
	}

	toInstall := make([]DependencyInfo, 0, len(indices))

	for j, depIdx := range indices {
		if installed[j] {
			continue
		}

		toInstall = append(toInstall, deps[depIdx])
	}

	return toInstall, nil
}

func (c *InstallCommand) shouldInstallDep(dep DependencyInfo) bool {
	// If user explicitly named this dependency, always allow it
	if c.TargetDep != "" && c.TargetDep == dep.Name {
		return true
	}

	// Required deps (enabled=true) are always installed
	if dep.Enabled == enabledTrue {
		return true
	}

	// Optional deps (enabled=auto) are installed only with --include-optional
	// Disabled deps (enabled=false) are never installed in bulk mode
	if dep.Enabled == enabledAuto && c.IncludeOptional {
		return true
	}

	return false
}

func (c *InstallCommand) isAlreadyInstalled(ctx context.Context, dep DependencyInfo) (bool, error) {
	sub, err := getSubscription(ctx, c.client.OLM(), dep.Namespace, dep.Subscription)
	if err != nil {
		return false, fmt.Errorf("check if %s is installed: %w", dep.Name, err)
	}

	if sub != nil {
		// Subscription exists - verify it matches expected spec
		c.warnIfSubscriptionMismatch(sub, dep)

		return true, nil
	}

	// No subscription - check for orphaned CSV (installed but subscription deleted)
	return c.hasSucceededCSV(ctx, dep.Namespace, dep.Subscription)
}

func (c *InstallCommand) hasSucceededCSV(ctx context.Context, namespace, subName string) (bool, error) {
	csvList, err := c.client.OLM().ClusterServiceVersions(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		// meta.IsNoMatchError returns true if the CRD doesn't exist (OLM not installed)
		if meta.IsNoMatchError(err) {
			return false, nil
		}

		return false, fmt.Errorf("list CSVs in %s: %w", namespace, err)
	}

	for i := range csvList.Items {
		csv := &csvList.Items[i]

		if csv.Status.Phase != operatorsv1alpha1.CSVPhaseSucceeded {
			continue
		}

		if MatchesSubscription(csv.Name, subName) {
			return true, nil
		}
	}

	return false, nil
}

func (c *InstallCommand) warnIfSubscriptionMismatch(sub *operatorsv1alpha1.Subscription, dep DependencyInfo) {
	expectedSource := dep.Source
	if expectedSource == "" {
		expectedSource = defaultCatalogSource
	}

	var mismatches []string

	if sub.Spec.Channel != dep.Channel {
		mismatches = append(mismatches, fmt.Sprintf("channel: %s (expected %s)", sub.Spec.Channel, dep.Channel))
	}

	if sub.Spec.CatalogSource != expectedSource {
		mismatches = append(mismatches, fmt.Sprintf("source: %s (expected %s)", sub.Spec.CatalogSource, expectedSource))
	}

	if len(mismatches) > 0 {
		w := c.IO.Out()
		_, _ = fmt.Fprintf(w, "  Warning: %s subscription has different spec: %s\n", dep.DisplayName, strings.Join(mismatches, ", "))
	}
}

func (c *InstallCommand) createDepResources(ctx context.Context, dep DependencyInfo) error {
	w := c.IO.Out()

	_, _ = fmt.Fprintf(w, msgInstalling, dep.DisplayName)
	_, _ = fmt.Fprintf(w, msgCreatingNamespace, dep.Namespace)

	if err := c.createNamespace(ctx, dep.Namespace); err != nil {
		return err
	}

	_, _ = fmt.Fprint(w, msgEnsuringOperatorGroup)

	created, err := c.ensureOperatorGroup(ctx, dep.Namespace, dep.TargetNamespaces)
	if err != nil {
		return err
	}

	if created {
		_, _ = fmt.Fprint(w, msgCreatedOperatorGroup)
	} else {
		_, _ = fmt.Fprint(w, msgOperatorGroupExists)
	}

	_, _ = fmt.Fprintln(w, msgCreatingSubscription)

	if err := c.createSubscription(ctx, dep); err != nil {
		return err
	}

	return nil
}

func (c *InstallCommand) createNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				labelManagedBy: labelManagedByValue,
			},
		},
	}

	unstructuredNS, err := toUnstructured(ns)
	if err != nil {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}

	_, err = c.client.Dynamic().Resource(corev1.SchemeGroupVersion.WithResource("namespaces")).
		Create(ctx, unstructuredNS, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}

	return nil
}

func (c *InstallCommand) ensureOperatorGroup(ctx context.Context, namespace string, targetNamespaces []string) (bool, error) {
	gvr := operatorsv1.SchemeGroupVersion.WithResource("operatorgroups")

	// Check if any OperatorGroup already exists in the namespace
	list, err := c.client.Dynamic().Resource(gvr).Namespace(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("list operatorgroups: %w", err)
	}

	// If an OperatorGroup already exists, check if TargetNamespaces match
	if len(list.Items) > 0 {
		existing := &list.Items[0]
		existingTargets, _, _ := unstructured.NestedStringSlice(existing.Object, "spec", "targetNamespaces")

		if !slicesEqualUnordered(existingTargets, targetNamespaces) {
			return false, fmt.Errorf(
				"operatorgroup %s/%s exists with targetNamespaces %v, but requested %v",
				namespace, existing.GetName(), existingTargets, targetNamespaces,
			)
		}

		return false, nil
	}

	og := &operatorsv1.OperatorGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "operators.coreos.com/v1",
			Kind:       "OperatorGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace + "-og",
			Namespace: namespace,
			Labels: map[string]string{
				labelManagedBy: labelManagedByValue,
			},
		},
		Spec: operatorsv1.OperatorGroupSpec{
			TargetNamespaces: targetNamespaces,
		},
	}

	unstructuredOG, err := toUnstructured(og)
	if err != nil {
		return false, fmt.Errorf("create operatorgroup: %w", err)
	}

	_, err = c.client.Dynamic().Resource(gvr).Namespace(namespace).
		Create(ctx, unstructuredOG, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return false, nil // Another actor created it between List and Create
		}

		return false, fmt.Errorf("create operatorgroup: %w", err)
	}

	return true, nil
}

func (c *InstallCommand) createSubscription(ctx context.Context, dep DependencyInfo) error {
	source := dep.Source
	if source == "" {
		source = defaultCatalogSource
	}

	sub := &operatorsv1alpha1.Subscription{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "operators.coreos.com/v1alpha1",
			Kind:       "Subscription",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Subscription,
			Namespace: dep.Namespace,
			Labels: map[string]string{
				labelManagedBy: labelManagedByValue,
			},
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			Channel:                dep.Channel,
			Package:                dep.Subscription,
			CatalogSource:          source,
			CatalogSourceNamespace: sourceNamespace,
			InstallPlanApproval:    operatorsv1alpha1.ApprovalAutomatic,
		},
	}

	gvr := operatorsv1alpha1.SchemeGroupVersion.WithResource("subscriptions")

	unstructuredSub, err := toUnstructured(sub)
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}

	_, err = c.client.Dynamic().Resource(gvr).Namespace(dep.Namespace).
		Create(ctx, unstructuredSub, metav1.CreateOptions{})
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create subscription: %w", err)
		}

		// Subscription already exists - verify it matches expected spec
		return c.verifyExistingSubscription(ctx, dep, source)
	}

	return nil
}

func (c *InstallCommand) verifyExistingSubscription(ctx context.Context, dep DependencyInfo, expectedSource string) error {
	existing, err := c.client.OLM().Subscriptions(dep.Namespace).Get(ctx, dep.Subscription, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get existing subscription: %w", err)
	}

	var mismatches []string

	if existing.Spec.Channel != dep.Channel {
		mismatches = append(mismatches, fmt.Sprintf("channel: %s (expected %s)", existing.Spec.Channel, dep.Channel))
	}

	if existing.Spec.CatalogSource != expectedSource {
		mismatches = append(mismatches, fmt.Sprintf("source: %s (expected %s)", existing.Spec.CatalogSource, expectedSource))
	}

	if len(mismatches) > 0 {
		w := c.IO.Out()
		_, _ = fmt.Fprintf(w, "  Warning: Subscription exists with different spec: %s\n", strings.Join(mismatches, ", "))
	}

	return nil
}

// csvResult holds the result of a CSV check.
type csvResult struct {
	version string
	ready   bool
	err     error
	printed bool // true if a status message was already printed
}

// tryCSVFromSubscription returns a CSV version when the subscription points at a succeeded CSV.
func (c *InstallCommand) tryCSVFromSubscription(
	ctx context.Context,
	w io.Writer,
	namespace string,
	subName string,
	startTime time.Time,
) csvResult {
	sub, err := getSubscription(ctx, c.client.OLM(), namespace, subName)
	if err != nil {
		return csvResult{err: fmt.Errorf("get subscription: %w", err)}
	}

	// Subscription not found yet - still waiting
	if sub == nil {
		return csvResult{ready: false}
	}

	// Check for subscription-level resolution failures
	for _, cond := range sub.Status.Conditions {
		if cond.Type == "ResolutionFailed" && cond.Status == "True" {
			return csvResult{err: fmt.Errorf("subscription resolution failed: %s", cond.Message)}
		}
	}

	if sub.Status.InstalledCSV == "" {
		return csvResult{ready: false}
	}

	csv, err := c.client.OLM().ClusterServiceVersions(namespace).Get(ctx, sub.Status.InstalledCSV, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return csvResult{ready: false}
		}

		return csvResult{err: fmt.Errorf("get CSV %s: %w", sub.Status.InstalledCSV, err)}
	}

	//nolint:exhaustive // default case handles all other phases
	switch csv.Status.Phase {
	case operatorsv1alpha1.CSVPhaseSucceeded:
		return csvResult{version: csv.Spec.Version.String(), ready: true}

	case operatorsv1alpha1.CSVPhaseFailed:
		// Terminal failure - don't keep polling
		reason := csv.Status.Reason
		if reason == "" {
			reason = "unknown"
		}

		return csvResult{err: fmt.Errorf("CSV %s failed: %s", csv.Name, reason)}

	default:
		// Pending, Installing, InstallReady, Unknown, Replacing, Deleting - keep polling
		elapsed := time.Since(startTime).Round(time.Second)
		_, _ = fmt.Fprintf(w, msgWaitingForCSVPhase, csv.Status.Phase, elapsed)

		return csvResult{ready: false, printed: true}
	}
}

// findSucceededCSVInNamespace lists CSVs and returns a version when a succeeded CSV matches the subscription.
func (c *InstallCommand) findSucceededCSVInNamespace(ctx context.Context, namespace string, subName string) csvResult {
	csvList, err := c.client.OLM().ClusterServiceVersions(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return csvResult{err: fmt.Errorf("list CSVs in %s: %w", namespace, err)}
	}

	for i := range csvList.Items {
		csv := &csvList.Items[i]

		if csv.Status.Phase != operatorsv1alpha1.CSVPhaseSucceeded {
			continue
		}

		if MatchesSubscription(csv.Name, subName) {
			return csvResult{version: csv.Spec.Version.String(), ready: true}
		}
	}

	return csvResult{ready: false}
}

func (c *InstallCommand) waitForCSV(ctx context.Context, w io.Writer, namespace, subName string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	startTime := time.Now()
	pollCount := 0

	for {
		pollCount++

		result := c.tryCSVFromSubscription(ctx, w, namespace, subName, startTime)
		if result.err != nil {
			return "", result.err
		}

		if result.ready {
			return result.version, nil
		}

		// Every Nth poll, also check for existing CSVs in the namespace
		if pollCount%csvNamespaceListEveryNPolls == 0 {
			prevPrinted := result.printed
			result = c.findSucceededCSVInNamespace(ctx, namespace, subName)
			result.printed = prevPrinted || result.printed

			if result.err != nil {
				return "", result.err
			}

			if result.ready {
				return result.version, nil
			}
		}

		// Only print generic message if tryCSVFromSubscription didn't print a phase-specific one
		if !result.printed {
			elapsed := time.Since(startTime).Round(time.Second)
			_, _ = fmt.Fprintf(w, msgWaitingForCSV, elapsed)
		}

		select {
		case <-ctx.Done():
			elapsed := time.Since(startTime).Round(time.Second)

			return "", fmt.Errorf("timeout waiting for CSV after %s: %w", elapsed, ctx.Err())
		case <-ticker.C:
		}
	}
}

// MatchesSubscription checks if a CSV name matches the subscription package.
// CSV names follow the pattern "<package>.v<version>" (e.g., "kueue-operator.v0.10.0").
func MatchesSubscription(csvName, subName string) bool {
	csvLower := strings.ToLower(csvName)
	subLower := strings.ToLower(subName)

	// Exact prefix match: "kueue-operator.v" for subscription "kueue-operator"
	if strings.HasPrefix(csvLower, subLower+".v") {
		return true
	}

	// Handle "openshift-" prefix: subscription "openshift-cert-manager-operator" -> CSV "cert-manager-operator.v1.19.0"
	subWithoutPrefix := strings.TrimPrefix(subLower, "openshift-")
	if subWithoutPrefix != subLower && strings.HasPrefix(csvLower, subWithoutPrefix+".v") {
		return true
	}

	// Try adding "-operator" suffix for subscriptions that don't have it
	// e.g., subscription "cert-manager" -> CSV "cert-manager-operator.v1.0.0"
	if !strings.HasSuffix(subWithoutPrefix, "-operator") && !strings.HasSuffix(subWithoutPrefix, "operator") {
		if strings.HasPrefix(csvLower, subWithoutPrefix+"-operator.v") {
			return true
		}
	}

	// Handle Red Hat naming: subscription "-product" -> CSV "-operator"
	// e.g., subscription "opentelemetry-product" -> CSV "opentelemetry-operator.v0.144.0-2"
	if subBase, found := strings.CutSuffix(subWithoutPrefix, "-product"); found {
		if strings.HasPrefix(csvLower, subBase+"-operator.v") {
			return true
		}
	}

	// Extract package part from CSV (before ".v")
	idx := strings.Index(csvLower, ".v")
	if idx <= 0 {
		return false // No version suffix, not a valid CSV name pattern
	}

	csvPackage := csvLower[:idx]

	// Compare normalized forms (hyphens removed) for strict equality
	// This handles variations like "jobset-operator" vs "job-set-operator"
	csvNorm := strings.ReplaceAll(csvPackage, "-", "")
	subNorm := strings.ReplaceAll(subWithoutPrefix, "-", "")

	return csvNorm == subNorm
}

func (c *InstallCommand) printSuccess(w io.Writer, name, version string) {
	if c.useColor {
		_, _ = fmt.Fprintf(w, colorGreen+msgSuccessInstall+colorReset, name, version)
	} else {
		_, _ = fmt.Fprintf(w, msgSuccessInstall, name, version)
	}
}

func (c *InstallCommand) printFailure(w io.Writer, name string, err error) {
	if c.useColor {
		_, _ = fmt.Fprintf(w, colorRed+msgFailInstall+colorReset, name, err)
	} else {
		_, _ = fmt.Fprintf(w, msgFailInstall, name, err)
	}
}

func (c *InstallCommand) printSummary(w io.Writer, results []InstallResult) {
	var installed, failed int

	var failedNames []string

	for _, r := range results {
		switch r.Status {
		case InstallStatusInstalled:
			installed++
		case InstallStatusFailed:
			failed++
			failedNames = append(failedNames, r.DisplayName)
		}
	}

	_, _ = fmt.Fprintln(w)

	summary := fmt.Sprintf("Summary: %d installed", installed)
	if failed > 0 {
		summary += fmt.Sprintf(", %d failed (%s)", failed, strings.Join(failedNames, ", "))
	}

	_, _ = fmt.Fprintln(w, summary)
}

// toUnstructured converts a typed object to unstructured.
func toUnstructured(obj any) (*unstructured.Unstructured, error) {
	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("convert to unstructured: %w", err)
	}

	return &unstructured.Unstructured{Object: data}, nil
}

// slicesEqualUnordered reports whether two string slices contain the same elements
// regardless of order. Duplicates are handled via a count map so
// ["a","a","b"] ≠ ["a","b"] as expected.
func slicesEqualUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}

	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}

	return true
}
