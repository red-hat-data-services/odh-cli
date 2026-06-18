package raycluster

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// PostUpgradeResult holds counts after post-upgrade.
type PostUpgradeResult struct {
	Migrated int
	Skipped  int
	Failed   int
}

// routeWaitInterval is the delay between route polling attempts.
const routeWaitInterval = 2 * time.Second

// deletionWaitMaxAttempts and deletionWaitInterval match the deletion poll loop (same as original 60 * 2s).
const deletionWaitMaxAttempts = 60
const deletionWaitInterval = 2 * time.Second

// healthCheckWaitInterval is the delay between health status polling attempts.
const healthCheckWaitInterval = 2 * time.Second

// waitForDeletion polls until the cluster is not found or ctx is cancelled.
// It logs "Waiting for cluster deletion to complete..." and "Cluster deleted successfully" via io.
// Returns (true, nil) when deleted, (false, err) on timeout or context cancel.
func waitForDeletion(ctx context.Context, dyn dynamic.NamespaceableResourceInterface, ns, name string, io iostreams.Interface) (bool, error) {
	io.Errorf("  [%s] Waiting for cluster deletion to complete...", name)
	for range deletionWaitMaxAttempts {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("context cancelled waiting for deletion of %s: %w", name, ctx.Err())
		default:
		}
		_, getErr := dyn.Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			io.Errorf("  [%s] Cluster deleted successfully", name)

			return true, nil
		}
		if getErr != nil {
			return false, fmt.Errorf("checking deletion status of %s: %w", name, getErr)
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("context cancelled waiting for deletion of %s: %w", name, ctx.Err())
		case <-time.After(deletionWaitInterval):
		}
	}

	return false, errors.New("timeout waiting for cluster deletion")
}

// PostUpgradeOptions bundles configuration for PostUpgrade.
type PostUpgradeOptions struct {
	ClusterName  string
	Namespace    string
	DryRun       bool
	SkipConfirm  bool
	FromBackup   string
	RouteTimeout time.Duration
}

// PostUpgrade runs post-upgrade migration: either live (in-place update) or from backup (delete + create).
func PostUpgrade(ctx context.Context, c client.Client, popts PostUpgradeOptions, io iostreams.Interface) (PostUpgradeResult, error) {
	routeTimeout := popts.RouteTimeout
	if routeTimeout <= 0 {
		routeTimeout = DefaultRouteTimeout
	}
	opts := migrateOptions{dryRun: popts.DryRun, skipConfirm: popts.SkipConfirm, routeTimeout: routeTimeout}
	if popts.FromBackup != "" {
		return postUpgradeFromBackup(ctx, c, popts.FromBackup, popts.ClusterName, popts.Namespace, opts, io)
	}

	return postUpgradeLive(ctx, c, popts.ClusterName, popts.Namespace, opts, io)
}

// waitForClusterRoute polls GetClusterRoute until a non-empty URL is returned, the timeout expires,
// or ctx is cancelled. Returns the dashboard URL or "" on timeout/cancellation.
func waitForClusterRoute(ctx context.Context, c client.Client, name, ns string, timeout time.Duration, io iostreams.Interface) string {
	maxAttempts := max(int(timeout/routeWaitInterval), 1)
	io.Errorf("  [%s] Waiting for cluster route to become available...", name)
	for range maxAttempts {
		select {
		case <-ctx.Done():
			return ""
		default:
		}
		url := GetClusterRoute(ctx, c, name, ns)
		if url != "" {
			return url
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(routeWaitInterval):
		}
	}

	return ""
}

func getClusterState(ctx context.Context, c client.Client, name, ns string) string {
	rc, err := c.Get(ctx, resources.RayCluster.GVR(), name, client.InNamespace(ns))
	if err != nil {
		return ""
	}

	state, _ := jq.Query[string](rc, ".status.state")

	return state
}

func waitForClusterReady(ctx context.Context, c client.Client, name, ns string, timeout time.Duration, io iostreams.Interface) string {
	maxAttempts := max(int(timeout/healthCheckWaitInterval), 1)
	io.Errorf("  [%s] Waiting for cluster pods to become healthy...", name)

	for range maxAttempts {
		select {
		case <-ctx.Done():
			return ""
		default:
		}

		state := getClusterState(ctx, c, name, ns)
		if state == stateReady {
			return state
		}

		select {
		case <-ctx.Done():
			return ""
		case <-time.After(healthCheckWaitInterval):
		}
	}

	return getClusterState(ctx, c, name, ns)
}

func clusterStateMessage(state string) string {
	if state == stateReady {
		return stateReady
	}

	if state == "" {
		return "not yet ready (timeout)"
	}

	return state + " (not yet ready)"
}

type migrateOptions struct {
	dryRun       bool
	skipConfirm  bool
	routeTimeout time.Duration
}

func postUpgradeLive(
	ctx context.Context,
	c client.Client,
	clusterName string,
	namespace string,
	opts migrateOptions,
	io iostreams.Interface,
) (PostUpgradeResult, error) {
	if clusterName != "" && namespace == "" {
		return PostUpgradeResult{}, errors.New("namespace must be specified when migrating a specific cluster")
	}

	scopeMsg := formatScopeMsg(clusterName, namespace)

	if opts.dryRun {
		io.Errorf("=== DRY RUN MODE - No changes will be made ===")
		io.Errorf("")
	}

	io.Errorf("Fetching RayClusters (%s)...", scopeMsg)
	clusters, err := GetClusters(ctx, c, clusterName, namespace)
	if err != nil {
		return PostUpgradeResult{}, err
	}
	if len(clusters) == 0 {
		io.Errorf("No RayClusters found to migrate")

		return PostUpgradeResult{Migrated: 0, Skipped: 0, Failed: 0}, nil
	}

	toMigrate, alreadyMigrated := analyzeClusters(clusters, io)

	io.Errorf("")
	io.Errorf("Summary: %d to migrate, %d already migrated", len(toMigrate), len(alreadyMigrated))

	if len(toMigrate) == 0 {
		io.Errorf("")
		io.Errorf("All clusters are already migrated. Nothing to do.")

		return PostUpgradeResult{Skipped: len(alreadyMigrated)}, nil
	}

	if !confirmLiveMigration(toMigrate, clusterName, namespace, opts, io) {
		return PostUpgradeResult{Skipped: len(alreadyMigrated)}, nil
	}

	migrated, failed := executeLiveMigration(ctx, c, toMigrate, opts, io)

	io.Errorf("")
	io.Errorf("============================================================")
	io.Errorf("Migration Summary:")
	io.Errorf("  Migrated: %d", migrated)
	io.Errorf("  Skipped (already migrated): %d", len(alreadyMigrated))
	io.Errorf("  Failed: %d", failed)

	return PostUpgradeResult{Migrated: migrated, Skipped: len(alreadyMigrated), Failed: failed}, nil
}

func formatScopeMsg(clusterName, namespace string) string {
	if clusterName != "" && namespace != "" {
		return fmt.Sprintf("cluster '%s' in namespace '%s'", clusterName, namespace)
	}
	if namespace != "" {
		return fmt.Sprintf("all clusters in namespace '%s'", namespace)
	}

	return "all clusters across all namespaces"
}

func analyzeClusters(clusters []*unstructured.Unstructured, io iostreams.Interface) (toMigrate, alreadyMigrated []*unstructured.Unstructured) { //nolint:nonamedreturns // named for clarity per confusing-results
	io.Errorf("Found %d RayCluster(s)", len(clusters))
	io.Errorf("")
	io.Errorf("Analyzing clusters for migration status...")

	total := len(clusters)
	for idx, rc := range clusters {
		name := rc.GetName()
		ns := rc.GetNamespace()
		if ns == "" {
			ns = DefaultNamespace
		}
		isMigrated, _ := IsClusterMigrated(rc)
		if isMigrated {
			alreadyMigrated = append(alreadyMigrated, rc)
			io.Errorf("  [%d/%d] Checking %s (ns: %s)... already migrated", idx+1, total, name, ns)
		} else {
			toMigrate = append(toMigrate, rc)
			io.Errorf("  [%d/%d] Checking %s (ns: %s)... needs migration", idx+1, total, name, ns)
		}
	}

	return toMigrate, alreadyMigrated
}

func confirmLiveMigration(toMigrate []*unstructured.Unstructured, clusterName, namespace string, opts migrateOptions, io iostreams.Interface) bool {
	if opts.dryRun || opts.skipConfirm {
		return true
	}

	if clusterName == "" {
		io.Errorf("")
		io.Errorf("============================================================")
		if namespace != "" {
			io.Errorf("WARNING: You are about to migrate ALL clusters in namespace '%s'", namespace)
		} else {
			io.Errorf("WARNING: You are about to migrate ALL clusters across ALL namespaces")
		}
		io.Errorf("============================================================")
	}
	io.Errorf("")
	io.Errorf("The following %d cluster(s) will be migrated:", len(toMigrate))
	for _, rc := range toMigrate {
		ns := rc.GetNamespace()
		if ns == "" {
			ns = DefaultNamespace
		}
		io.Errorf("  - %s (namespace: %s)", rc.GetName(), ns)
	}
	io.Errorf("")
	io.Errorf("IMPORTANT: Migration will cause temporary downtime for each RayCluster.")
	io.Errorf("  - Pods will be restarted as the KubeRay operator recreates them with the new configuration.")
	io.Errorf("  - Existing job state and logs will be lost.")
	io.Errorf("  - Currently running workloads/jobs will be interrupted and progress lost.")
	io.Errorf("")
	if !confirmation.Prompt(io, "Proceed with migration?") {
		io.Errorf("Migration cancelled.")

		return false
	}
	io.Errorf("")

	return true
}

func executeLiveMigration(ctx context.Context, c client.Client, toMigrate []*unstructured.Unstructured, opts migrateOptions, io iostreams.Interface) (migrated, failed int) { //nolint:nonamedreturns // named for clarity per confusing-results
	gvr := resources.RayCluster.GVR()
	dyn := c.Dynamic().Resource(gvr)

	for _, rc := range toMigrate {
		name := rc.GetName()
		ns := rc.GetNamespace()
		if ns == "" {
			ns = DefaultNamespace
		}

		if opts.dryRun {
			io.Errorf("  [DRY RUN] Would migrate: %s (ns: %s)", name, ns)
			migrated++

			continue
		}

		if err := migrateSingleClusterLive(ctx, c, dyn, name, ns, opts.routeTimeout, io); err != nil {
			io.Errorf("  [FAIL] %s (ns: %s): %v", name, ns, err)
			failed++

			continue
		}

		migrated++
	}

	return migrated, failed
}

func migrateSingleClusterLive(ctx context.Context, c client.Client, dyn dynamic.NamespaceableResourceInterface, name, ns string, routeTimeout time.Duration, io iostreams.Interface) error {
	io.Errorf("  [%s] Fetching current cluster state...", name)
	latest, err := dyn.Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("fetching cluster %s: %w", name, err)
	}

	cleaned := latest.DeepCopy()
	RemoveAutogeneratedFields(cleaned.Object)
	ProcessRayClusterYAML(cleaned)
	if rv := latest.GetResourceVersion(); rv != "" {
		cleaned.SetResourceVersion(rv)
	}
	if uid := latest.GetUID(); uid != "" {
		cleaned.SetUID(uid)
	}

	io.Errorf("  [%s] Cleaning up old CodeFlare ServiceAccounts...", name)
	cleanupOldServiceAccounts(ctx, c, ns, name, io)

	io.Errorf("  [%s] Applying migration changes...", name)
	_, err = dyn.Namespace(ns).Update(ctx, cleaned, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating cluster %s: %w", name, err)
	}

	state := waitForClusterReady(ctx, c, name, ns, routeTimeout, io)
	url := waitForClusterRoute(ctx, c, name, ns, routeTimeout, io)

	io.Errorf("  [OK] Migrated: %s (ns: %s)", name, ns)
	io.Errorf("       Status: %s", clusterStateMessage(state))

	if url != "" {
		io.Errorf("       Dashboard: %s", url)
	} else {
		io.Errorf("       Dashboard: route not yet available (timeout)")
	}

	return nil
}

func cleanupOldServiceAccounts(ctx context.Context, c client.Client, namespace, clusterName string, io iostreams.Interface) {
	sas, err := c.List(ctx, resources.ServiceAccount, client.WithNamespace(namespace))
	if err != nil {
		return
	}
	prefix := clusterName + "-oauth-proxy-"
	kuberaySA := clusterName + "-oauth-proxy-sa"
	for _, sa := range sas {
		name := sa.GetName()
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix && name != kuberaySA {
			_ = c.Dynamic().Resource(resources.ServiceAccount.GVR()).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
			io.Errorf("  [%s] Deleting old ServiceAccount: %s", clusterName, name)
		}
	}
}

type backupItem struct {
	u    *unstructured.Unstructured
	file string
}

func postUpgradeFromBackup(
	ctx context.Context,
	c client.Client,
	backupPath string,
	clusterName string,
	namespace string,
	opts migrateOptions,
	io iostreams.Interface,
) (PostUpgradeResult, error) {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return PostUpgradeResult{}, fmt.Errorf("backup path does not exist: %s", backupPath)
	}

	yamlFiles := collectYAMLFiles(backupPath, io)
	if len(yamlFiles) == 0 {
		reportNoYAMLFiles(backupPath, io)

		return PostUpgradeResult{}, nil
	}

	toApply := parseBackupItems(yamlFiles, clusterName, namespace, io)
	if len(toApply) == 0 {
		io.Errorf("No matching RayClusters found in backup")

		return PostUpgradeResult{}, nil
	}

	scopeMsg := formatScopeMsg(clusterName, namespace)

	if opts.dryRun {
		io.Errorf("=== DRY RUN MODE - No changes will be made ===")
		io.Errorf("")
	}

	io.Errorf("Found %d RayCluster(s) in backup to migrate (%s):", len(toApply), scopeMsg)
	io.Errorf("")
	for _, item := range toApply {
		name := item.u.GetName()
		ns := item.u.GetNamespace()
		if ns == "" {
			ns = DefaultNamespace
		}
		io.Errorf("  - %s (ns: %s) from %s", name, ns, filepath.Base(item.file))
	}

	if !confirmBackupRestore(opts, io) {
		return PostUpgradeResult{}, nil
	}

	migrated, failed := executeBackupRestore(ctx, c, toApply, opts, io)

	io.Errorf("")
	io.Errorf("============================================================")
	if opts.dryRun {
		io.Errorf("DRY RUN Summary:")
		io.Errorf("  Would restore: %d", migrated)
	} else {
		io.Errorf("Restore from Backup Summary:")
		io.Errorf("  Restored: %d", migrated)
	}
	io.Errorf("  Failed: %d", failed)

	return PostUpgradeResult{Migrated: migrated, Failed: failed}, nil
}

func isYAMLFile(name string) bool {
	ext := filepath.Ext(name)

	return ext == yamlExt || ext == ymlExt
}

func collectYAMLFiles(backupPath string, io iostreams.Interface) []string {
	info, _ := os.Stat(backupPath)
	if info != nil && info.Mode().IsRegular() {
		if !isYAMLFile(backupPath) {
			return nil
		}

		return []string{backupPath}
	}

	yamlFiles := collectYAMLFromDir(backupPath)
	if len(yamlFiles) == 0 {
		rhoai3 := filepath.Join(backupPath, BackupSubdirRHOAI3x)
		if st, _ := os.Stat(rhoai3); st != nil && st.IsDir() {
			io.Errorf("No YAML files in '%s', using '%s' subdirectory...", backupPath, rhoai3)
			yamlFiles = collectYAMLFromDir(rhoai3)
		}
	}

	return yamlFiles
}

func collectYAMLFromDir(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if isYAMLFile(e.Name()) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}

	return files
}

func parseBackupItems(yamlFiles []string, clusterName, namespace string, io iostreams.Interface) []backupItem {
	var items []backupItem
	for _, f := range yamlFiles {
		item, ok := parseOneBackupFile(f, clusterName, namespace, io)
		if ok {
			items = append(items, item)
		}
	}

	return items
}

func parseOneBackupFile(f, clusterName, namespace string, io iostreams.Interface) (backupItem, bool) {
	data, err := os.ReadFile(filepath.Clean(f))
	if err != nil {
		io.Errorf("failed to read file %s: %v", f, err)

		return backupItem{}, false
	}
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		io.Errorf("failed to parse YAML %s: %v", f, err)

		return backupItem{}, false
	}
	kind, _ := obj["kind"].(string)
	if kind != "RayCluster" {
		return backupItem{}, false
	}
	meta, _ := obj["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	ns, _ := meta["namespace"].(string)
	if ns == "" {
		ns = DefaultNamespace
	}
	if clusterName != "" && name != clusterName {
		return backupItem{}, false
	}
	if namespace != "" && ns != namespace {
		return backupItem{}, false
	}
	u := &unstructured.Unstructured{Object: obj}

	return backupItem{u: u, file: f}, true
}

func reportNoYAMLFiles(backupPath string, io iostreams.Interface) {
	io.Errorf("No YAML files found in: %s", backupPath)
	info, _ := os.Stat(backupPath)
	if info == nil || !info.IsDir() {
		return
	}
	entries, _ := os.ReadDir(backupPath)
	var subdirs []string
	for _, e := range entries {
		if e.IsDir() {
			subdirs = append(subdirs, e.Name())
		}
	}
	if len(subdirs) > 0 {
		io.Errorf("Found subdirectories: %s", strings.Join(subdirs, ", "))
		io.Errorf("Hint: Use --from-backup %s/rhoai-3.x for RHOAI 3.x migration", backupPath)
		io.Errorf("      or --from-backup %s/rhoai-2.x for RHOAI 2.x rollback", backupPath)
	}
}

func confirmBackupRestore(opts migrateOptions, io iostreams.Interface) bool {
	if opts.dryRun || opts.skipConfirm {
		return true
	}

	io.Errorf("")
	io.Errorf("WARNING: Restore from backup will DELETE and RECREATE each RayCluster.")
	io.Errorf("  - If a cluster currently exists, it will be deleted first.")
	io.Errorf("  - All running pods, jobs, and workloads will be terminated.")
	io.Errorf("  - Existing job state and logs will be lost.")
	io.Errorf("  - The cluster will be recreated from the backup configuration.")
	io.Errorf("")
	if !confirmation.Prompt(io, "Proceed with restore from backup?") {
		io.Errorf("Restore cancelled.")

		return false
	}
	io.Errorf("")

	return true
}

func executeBackupRestore(ctx context.Context, c client.Client, toApply []backupItem, opts migrateOptions, io iostreams.Interface) (migrated, failed int) { //nolint:nonamedreturns // named for clarity per confusing-results
	gvr := resources.RayCluster.GVR()
	dyn := c.Dynamic().Resource(gvr)

	for _, item := range toApply {
		u := item.u
		name := u.GetName()
		ns := u.GetNamespace()
		if ns == "" {
			ns = DefaultNamespace
		}

		if opts.dryRun {
			io.Errorf("  [DRY RUN] Would restore from backup: %s (ns: %s)", name, ns)
			io.Errorf("            (will delete existing cluster if present, then create from backup)")
			migrated++

			continue
		}

		if err := restoreSingleCluster(ctx, c, dyn, u, name, ns, opts.routeTimeout, io); err != nil {
			io.Errorf("  [FAIL] %s (ns: %s): %v", name, ns, err)
			failed++

			continue
		}

		migrated++
	}

	return migrated, failed
}

func restoreSingleCluster(ctx context.Context, c client.Client, dyn dynamic.NamespaceableResourceInterface, u *unstructured.Unstructured, name, ns string, routeTimeout time.Duration, io iostreams.Interface) error {
	_, err := dyn.Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("checking existing cluster %s: %w", name, err)
	}
	if err == nil {
		io.Errorf("  [%s] Deleting existing cluster...", name)
		if delErr := dyn.Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{}); delErr != nil {
			return fmt.Errorf("delete failed: %w", delErr)
		}
		deleted, delErr := waitForDeletion(ctx, dyn, ns, name, io)
		if !deleted {
			return delErr
		}
	}

	io.Errorf("  [%s] Creating cluster from backup...", name)
	RemoveKueueWorkloadAnnotations(u)
	u.SetResourceVersion("")
	u.SetUID("")
	_, err = dyn.Namespace(ns).Create(ctx, u, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating cluster %s: %w", name, err)
	}

	state := waitForClusterReady(ctx, c, name, ns, routeTimeout, io)
	url := waitForClusterRoute(ctx, c, name, ns, routeTimeout, io)

	io.Errorf("  [OK] Restored from backup: %s (ns: %s)", name, ns)
	io.Errorf("       Status: %s", clusterStateMessage(state))

	if url != "" {
		io.Errorf("       Dashboard: %s", url)
	} else {
		io.Errorf("       Dashboard: route not yet available (timeout)")
	}

	return nil
}
