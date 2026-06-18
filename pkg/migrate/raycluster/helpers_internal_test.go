package raycluster

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

// --- formatScopeMsg ---

func TestFormatScopeMsg_AllNamespaces(t *testing.T) {
	g := NewWithT(t)
	g.Expect(formatScopeMsg("", "")).To(Equal("all clusters across all namespaces"))
}

func TestFormatScopeMsg_SpecificNamespace(t *testing.T) {
	g := NewWithT(t)
	g.Expect(formatScopeMsg("", "my-ns")).To(Equal("all clusters in namespace 'my-ns'"))
}

func TestFormatScopeMsg_SpecificCluster(t *testing.T) {
	g := NewWithT(t)
	g.Expect(formatScopeMsg("my-cluster", "my-ns")).To(Equal("cluster 'my-cluster' in namespace 'my-ns'"))
}

// --- isYAMLFile ---

func TestIsYAMLFile(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"cluster.yaml", true},
		{"cluster.yml", true},
		{"cluster.json", false},
		{"cluster.txt", false},
		{"noext", false},
		{".yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(isYAMLFile(tt.name)).To(Equal(tt.expected))
		})
	}
}

// --- collectYAMLFromDir ---

func TestCollectYAMLFromDir_MixedFiles(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	for _, name := range []string{"a.yaml", "b.yml", "c.json", "d.txt"} {
		g.Expect(os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o600)).To(Succeed())
	}

	files := collectYAMLFromDir(dir)
	g.Expect(files).To(HaveLen(2))
	g.Expect(files).To(ContainElement(filepath.Join(dir, "a.yaml")))
	g.Expect(files).To(ContainElement(filepath.Join(dir, "b.yml")))
}

func TestCollectYAMLFromDir_Empty(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	files := collectYAMLFromDir(dir)
	g.Expect(files).To(BeEmpty())
}

func TestCollectYAMLFromDir_SkipsSubdirectories(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	g.Expect(os.Mkdir(filepath.Join(dir, "subdir"), 0o750)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(dir, "subdir", "inner.yaml"), []byte("test"), 0o600)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(dir, "outer.yaml"), []byte("test"), 0o600)).To(Succeed())

	files := collectYAMLFromDir(dir)
	g.Expect(files).To(HaveLen(1))
	g.Expect(files[0]).To(Equal(filepath.Join(dir, "outer.yaml")))
}

// --- collectYAMLFiles ---

func TestCollectYAMLFiles_SingleFile(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "cluster.yaml")
	g.Expect(os.WriteFile(f, []byte("test"), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)
	files := collectYAMLFiles(f, io)
	g.Expect(files).To(HaveLen(1))
	g.Expect(files[0]).To(Equal(f))
}

func TestCollectYAMLFiles_SingleNonYAMLFile(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "cluster.json")
	g.Expect(os.WriteFile(f, []byte("test"), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)
	files := collectYAMLFiles(f, io)
	g.Expect(files).To(BeEmpty())
}

func TestCollectYAMLFiles_FallsBackToRHOAI3xSubdir(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	rhoai3Dir := filepath.Join(dir, BackupSubdirRHOAI3x)
	g.Expect(os.MkdirAll(rhoai3Dir, 0o750)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(rhoai3Dir, "cluster.yaml"), []byte("test"), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)
	files := collectYAMLFiles(dir, io)
	g.Expect(files).To(HaveLen(1))
	g.Expect(errBuf.String()).To(ContainSubstring("rhoai-3.x"))
}

// --- parseOneBackupFile ---

func TestParseOneBackupFile_ValidRayCluster(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "raycluster.yaml")

	content := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: my-cluster
  namespace: my-ns
spec:
  headGroupSpec:
    enableIngress: false
`
	g.Expect(os.WriteFile(f, []byte(content), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	item, ok := parseOneBackupFile(f, "", "", io)
	g.Expect(ok).To(BeTrue())
	g.Expect(item.u.GetName()).To(Equal("my-cluster"))
	g.Expect(item.u.GetNamespace()).To(Equal("my-ns"))
	g.Expect(item.file).To(Equal(f))
}

func TestParseOneBackupFile_NonRayCluster(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "configmap.yaml")

	content := `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
  namespace: my-ns
`
	g.Expect(os.WriteFile(f, []byte(content), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	_, ok := parseOneBackupFile(f, "", "", io)
	g.Expect(ok).To(BeFalse())
}

func TestParseOneBackupFile_FilterByClusterName(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "raycluster.yaml")

	content := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: my-cluster
  namespace: my-ns
`
	g.Expect(os.WriteFile(f, []byte(content), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	_, ok := parseOneBackupFile(f, "other-cluster", "", io)
	g.Expect(ok).To(BeFalse())

	item, ok := parseOneBackupFile(f, "my-cluster", "", io)
	g.Expect(ok).To(BeTrue())
	g.Expect(item.u.GetName()).To(Equal("my-cluster"))
}

func TestParseOneBackupFile_FilterByNamespace(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "raycluster.yaml")

	content := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: my-cluster
  namespace: my-ns
`
	g.Expect(os.WriteFile(f, []byte(content), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	_, ok := parseOneBackupFile(f, "", "other-ns", io)
	g.Expect(ok).To(BeFalse())

	item, ok := parseOneBackupFile(f, "", "my-ns", io)
	g.Expect(ok).To(BeTrue())
	g.Expect(item.u.GetNamespace()).To(Equal("my-ns"))
}

func TestParseOneBackupFile_DefaultNamespace(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "raycluster.yaml")

	content := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: my-cluster
`
	g.Expect(os.WriteFile(f, []byte(content), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	item, ok := parseOneBackupFile(f, "", "", io)
	g.Expect(ok).To(BeTrue())
	g.Expect(item.u.GetNamespace()).To(Equal(""))
}

func TestParseOneBackupFile_InvalidYAML(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "bad.yaml")
	g.Expect(os.WriteFile(f, []byte("- :\n  :\n\t- broken"), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	_, ok := parseOneBackupFile(f, "", "", io)
	g.Expect(ok).To(BeFalse())
}

func TestParseOneBackupFile_MissingFile(t *testing.T) {
	g := NewWithT(t)
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	_, ok := parseOneBackupFile("/nonexistent/file.yaml", "", "", io)
	g.Expect(ok).To(BeFalse())
	g.Expect(errBuf.String()).To(ContainSubstring("failed to read"))
}

// --- analyzeClusters ---

func TestAnalyzeClusters_MixedStatus(t *testing.T) {
	g := NewWithT(t)

	migratedRC := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata": map[string]any{
				"name":      "migrated-cluster",
				"namespace": "ns1",
				"annotations": map[string]any{
					SecureNetworkAnnotation: "true",
				},
			},
			"spec": map[string]any{
				"headGroupSpec": map[string]any{
					"template": map[string]any{
						"spec": map[string]any{
							"containers": []any{
								map[string]any{"name": "ray-head"},
							},
						},
					},
				},
			},
		},
	}

	unmigrated := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata": map[string]any{
				"name":      "needs-migration",
				"namespace": "ns2",
			},
			"spec": map[string]any{
				"headGroupSpec": map[string]any{
					"enableIngress": true,
					"template": map[string]any{
						"spec": map[string]any{
							"containers": []any{
								map[string]any{"name": "ray-head"},
								map[string]any{"name": "oauth-proxy"},
							},
						},
					},
				},
			},
		},
	}

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	toMigrate, alreadyMigrated := analyzeClusters([]*unstructured.Unstructured{migratedRC, unmigrated}, io)

	g.Expect(toMigrate).To(HaveLen(1))
	g.Expect(toMigrate[0].GetName()).To(Equal("needs-migration"))

	g.Expect(alreadyMigrated).To(HaveLen(1))
	g.Expect(alreadyMigrated[0].GetName()).To(Equal("migrated-cluster"))
}

func TestAnalyzeClusters_AllMigrated(t *testing.T) {
	g := NewWithT(t)

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata": map[string]any{
				"name":      "cluster-1",
				"namespace": "ns1",
				"annotations": map[string]any{
					SecureNetworkAnnotation: "true",
				},
			},
			"spec": map[string]any{},
		},
	}

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	toMigrate, alreadyMigrated := analyzeClusters([]*unstructured.Unstructured{rc}, io)

	g.Expect(toMigrate).To(BeEmpty())
	g.Expect(alreadyMigrated).To(HaveLen(1))
}

// --- clusterInfoFrom ---

func TestClusterInfoFrom_FullCluster(t *testing.T) {
	g := NewWithT(t)

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata": map[string]any{
				"name":      "my-cluster",
				"namespace": "my-ns",
			},
			"spec": map[string]any{
				"headGroupSpec": map[string]any{
					"enableIngress": true,
					"template": map[string]any{
						"spec": map[string]any{
							"containers": []any{
								map[string]any{"name": "oauth-proxy"},
							},
						},
					},
				},
				"workerGroupSpecs": []any{
					map[string]any{
						"replicas": int64(3),
					},
				},
			},
			"status": map[string]any{
				"state": "ready",
			},
		},
	}

	info := clusterInfoFrom(rc)

	g.Expect(info.Name).To(Equal("my-cluster"))
	g.Expect(info.Namespace).To(Equal("my-ns"))
	g.Expect(info.Status).To(Equal("ready"))
	g.Expect(info.NumWorkers).To(Equal(int64(3)))
	g.Expect(info.Migrated).To(BeFalse())
	g.Expect(info.MigrationStatus).To(ContainSubstring("Needs migration"))
	g.Expect(info.TLSOAuthComponents).ToNot(BeEmpty())
}

func TestClusterInfoFrom_DefaultNamespace(t *testing.T) {
	g := NewWithT(t)

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata": map[string]any{
				"name": "my-cluster",
			},
			"spec": map[string]any{},
		},
	}

	info := clusterInfoFrom(rc)
	g.Expect(info.Namespace).To(Equal(DefaultNamespace))
	g.Expect(info.Status).To(Equal("unknown"))
	g.Expect(info.NumWorkers).To(Equal(int64(0)))
}

func TestClusterInfoFrom_MigratedCluster(t *testing.T) {
	g := NewWithT(t)

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata": map[string]any{
				"name":      "migrated",
				"namespace": "ns1",
				"annotations": map[string]any{
					SecureNetworkAnnotation: "true",
				},
			},
			"spec": map[string]any{},
			"status": map[string]any{
				"state": "running",
			},
		},
	}

	info := clusterInfoFrom(rc)
	g.Expect(info.Migrated).To(BeTrue())
	g.Expect(info.MigrationStatus).To(ContainSubstring("Already migrated"))
	g.Expect(info.TLSOAuthComponents).To(BeEmpty())
}

// --- reportPreflightChecks ---

func TestReportPreflightChecks_AllPassed(t *testing.T) {
	g := NewWithT(t)

	checks := []PreflightCheck{
		{Name: "Permissions", Passed: true, Message: "OK", Required: true},
		{Name: "cert-manager", Passed: true, Message: "found", Required: true},
	}

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	err := reportPreflightChecks(checks, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(errBuf.String()).To(ContainSubstring("[OK]"))
	g.Expect(errBuf.String()).To(ContainSubstring("All pre-upgrade checks passed"))
}

func TestReportPreflightChecks_RequiredFailure(t *testing.T) {
	g := NewWithT(t)

	checks := []PreflightCheck{
		{Name: "Permissions", Passed: true, Message: "OK", Required: true},
		{Name: "cert-manager", Passed: false, Message: "not found", Required: true, Help: "Install cert-manager"},
	}

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	err := reportPreflightChecks(checks, io)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("pre-upgrade checks failed"))
	g.Expect(errBuf.String()).To(ContainSubstring("[FAIL]"))
	g.Expect(errBuf.String()).To(ContainSubstring("Install cert-manager"))
}

func TestReportPreflightChecks_OptionalWarning(t *testing.T) {
	g := NewWithT(t)

	checks := []PreflightCheck{
		{Name: "Permissions", Passed: true, Message: "OK", Required: true},
		{Name: "optional-check", Passed: false, Message: "warning", Required: false},
	}

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	err := reportPreflightChecks(checks, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(errBuf.String()).To(ContainSubstring("completed with warnings"))
}

func TestReportPreflightChecks_WithDetails(t *testing.T) {
	g := NewWithT(t)

	checks := []PreflightCheck{
		{
			Name:     "Permissions",
			Passed:   false,
			Message:  "Missing permissions",
			Required: true,
			Details:  []string{"List namespaces: DENIED", "List RayClusters: DENIED"},
		},
	}

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	err := reportPreflightChecks(checks, io)
	g.Expect(err).To(HaveOccurred())
	g.Expect(errBuf.String()).To(ContainSubstring("List namespaces: DENIED"))
	g.Expect(errBuf.String()).To(ContainSubstring("List RayClusters: DENIED"))
}

// --- logBackupScope ---

func TestLogBackupScope(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		namespace   string
		count       int
		expected    string
	}{
		{"all namespaces", "", "", 5, "all clusters across all namespaces"},
		{"specific namespace", "", "my-ns", 3, "all clusters in namespace 'my-ns'"},
		{"specific cluster", "my-cluster", "my-ns", 1, "cluster 'my-cluster' in namespace 'my-ns'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var errBuf bytes.Buffer
			io := iostreams.NewIOStreams(nil, nil, &errBuf)
			logBackupScope(tt.clusterName, tt.namespace, tt.count, io)
			g.Expect(errBuf.String()).To(ContainSubstring(tt.expected))
		})
	}
}

// --- ensureBackupDirs ---

func TestEnsureBackupDirs_CreatesDirectories(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backups")

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	rhoai2, rhoai3, err := ensureBackupDirs(backupDir, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rhoai2).To(Equal(filepath.Join(backupDir, BackupSubdirRHOAI2x)))
	g.Expect(rhoai3).To(Equal(filepath.Join(backupDir, BackupSubdirRHOAI3x)))

	_, err2 := os.Stat(rhoai2)
	g.Expect(err2).ToNot(HaveOccurred())
	_, err3 := os.Stat(rhoai3)
	g.Expect(err3).ToNot(HaveOccurred())

	g.Expect(errBuf.String()).To(ContainSubstring("Created backup directory"))
}

func TestEnsureBackupDirs_ExistingDir(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	_, _, err := ensureBackupDirs(dir, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(errBuf.String()).ToNot(ContainSubstring("Created backup directory"))
}

// --- writeUnstructuredToFile / backupCluster ---

func TestBackupCluster_WritesFiles(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	rhoai2Dir := filepath.Join(dir, BackupSubdirRHOAI2x)
	rhoai3Dir := filepath.Join(dir, BackupSubdirRHOAI3x)
	g.Expect(os.MkdirAll(rhoai2Dir, 0o750)).To(Succeed())
	g.Expect(os.MkdirAll(rhoai3Dir, 0o750)).To(Succeed())

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion":        "ray.io/v1",
			"kind":              "RayCluster",
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"metadata": map[string]any{
				"name":      "test-cluster",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"headGroupSpec": map[string]any{
					"enableIngress": true,
					"template": map[string]any{
						"spec": map[string]any{
							"containers": []any{
								map[string]any{"name": "oauth-proxy"},
								map[string]any{"name": "ray-head"},
							},
						},
					},
				},
			},
		},
	}

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	f3x, err := backupCluster(rc, rhoai2Dir, rhoai3Dir, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(f3x).ToNot(BeEmpty())

	// Verify 2.x backup exists
	f2x := filepath.Join(rhoai2Dir, "raycluster-test-cluster-test-ns.yaml")
	_, err = os.Stat(f2x)
	g.Expect(err).ToNot(HaveOccurred())

	// Verify 3.x backup exists and has TLS components removed
	_, err = os.Stat(f3x)
	g.Expect(err).ToNot(HaveOccurred())

	data, err := os.ReadFile(filepath.Clean(f3x))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(data)).ToNot(ContainSubstring("oauth-proxy"))
	g.Expect(string(data)).To(ContainSubstring("ray-head"))

	g.Expect(errBuf.String()).To(ContainSubstring("Backed up: test-cluster"))
}

// --- printTable ---

func TestPrintTable(t *testing.T) {
	g := NewWithT(t)

	infos := []ClusterInfo{
		{Name: "cluster-a", Namespace: "ns1", Status: "ready", NumWorkers: 2, Migrated: true, MigrationStatus: "Already migrated"},
		{Name: "cluster-b", Namespace: "ns2", Status: "running", NumWorkers: 5, Migrated: false, MigrationStatus: "Needs migration"},
	}

	var outBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, &outBuf, nil)

	printTable(infos, io)

	out := outBuf.String()
	g.Expect(out).To(ContainSubstring("RayCluster Migration Status"))
	g.Expect(out).To(ContainSubstring("cluster-a"))
	g.Expect(out).To(ContainSubstring("cluster-b"))
	g.Expect(out).To(ContainSubstring("[MIGRATED]"))
	g.Expect(out).To(ContainSubstring("[NEEDS MIGRATION]"))
	g.Expect(out).To(ContainSubstring("1 migrated, 1 need migration"))
}

// --- confirmLiveMigration ---

func TestConfirmLiveMigration_DryRunSkipsPrompt(t *testing.T) {
	g := NewWithT(t)

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata":   map[string]any{"name": "test", "namespace": "ns"},
			"spec":       map[string]any{},
		},
	}

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	result := confirmLiveMigration([]*unstructured.Unstructured{rc}, "test", "ns",
		migrateOptions{dryRun: true}, io)
	g.Expect(result).To(BeTrue())
}

func TestConfirmLiveMigration_SkipConfirmSkipsPrompt(t *testing.T) {
	g := NewWithT(t)

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata":   map[string]any{"name": "test", "namespace": "ns"},
			"spec":       map[string]any{},
		},
	}

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	result := confirmLiveMigration([]*unstructured.Unstructured{rc}, "test", "ns",
		migrateOptions{skipConfirm: true}, io)
	g.Expect(result).To(BeTrue())
}

// --- confirmBackupRestore ---

func TestConfirmBackupRestore_DryRun(t *testing.T) {
	g := NewWithT(t)
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	result := confirmBackupRestore(migrateOptions{dryRun: true}, io)
	g.Expect(result).To(BeTrue())
}

func TestConfirmBackupRestore_SkipConfirm(t *testing.T) {
	g := NewWithT(t)
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	result := confirmBackupRestore(migrateOptions{skipConfirm: true}, io)
	g.Expect(result).To(BeTrue())
}

// --- waitForClusterRoute ---

func TestWaitForClusterRoute_CancelledContextReturnsEmpty(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	start := time.Now()
	url := waitForClusterRoute(ctx, nil, "test-cluster", "test-ns", 30*time.Second, io)
	elapsed := time.Since(start)

	g.Expect(url).To(BeEmpty())
	g.Expect(elapsed).To(BeNumerically("<", 2*time.Second))
}

func TestWaitForClusterRoute_ZeroTimeoutUsesMinimumOneAttempt(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	url := waitForClusterRoute(ctx, nil, "test", "ns", 0, io)
	g.Expect(url).To(BeEmpty())
	g.Expect(errBuf.String()).To(ContainSubstring("Waiting for cluster route to become available"))
}

func TestWaitForClusterRoute_NegativeTimeoutUsesMinimumOneAttempt(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	url := waitForClusterRoute(ctx, nil, "test", "ns", -5*time.Second, io)
	g.Expect(url).To(BeEmpty())
	g.Expect(errBuf.String()).To(ContainSubstring("Waiting for cluster route to become available"))
}

func TestWaitForClusterRoute_TimeoutComputesCorrectAttempts(t *testing.T) {
	g := NewWithT(t)

	g.Expect(max(int(10*time.Second/routeWaitInterval), 1)).To(Equal(5))
	g.Expect(max(int(DefaultRouteTimeout/routeWaitInterval), 1)).To(Equal(60))
	g.Expect(max(int(5*time.Minute/routeWaitInterval), 1)).To(Equal(150))
	g.Expect(max(int(1*time.Second/routeWaitInterval), 1)).To(Equal(1))
	g.Expect(max(int(0/routeWaitInterval), 1)).To(Equal(1))
}

// --- migrateOptions.routeTimeout ---

func TestMigrateOptions_RouteTimeoutField(t *testing.T) {
	g := NewWithT(t)

	opts := migrateOptions{routeTimeout: 5 * time.Minute}
	g.Expect(opts.routeTimeout).To(Equal(5 * time.Minute))

	opts = migrateOptions{}
	g.Expect(opts.routeTimeout).To(Equal(time.Duration(0)))
}

// --- waitForDeletion ---

func newFakeDynamic(t *testing.T) *dynamicfake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			resources.RayCluster.GVR(): resources.RayCluster.ListKind(),
		},
	)
}

func TestWaitForDeletion_ImmediateNotFound(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	dynResource := dyn.Resource(resources.RayCluster.GVR())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	deleted, err := waitForDeletion(t.Context(), dynResource, "ns", "cluster", io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deleted).To(BeTrue())
	g.Expect(errBuf.String()).To(ContainSubstring("Cluster deleted successfully"))
}

func TestWaitForDeletion_ContextCancelled(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	dynResource := dyn.Resource(resources.RayCluster.GVR())

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata":   map[string]any{"name": "cluster", "namespace": "ns"},
			"spec":       map[string]any{},
		},
	}
	_, err := dynResource.Namespace("ns").Create(t.Context(), rc, metav1.CreateOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	deleted, err := waitForDeletion(ctx, dynResource, "ns", "cluster", io)
	g.Expect(err).To(HaveOccurred())
	g.Expect(deleted).To(BeFalse())
	g.Expect(err.Error()).To(ContainSubstring("context cancelled"))
}

func TestWaitForDeletion_GetError(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	dynResource := dyn.Resource(resources.RayCluster.GVR())

	dyn.PrependReactor("get", "rayclusters", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("server error")
	})

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	deleted, err := waitForDeletion(t.Context(), dynResource, "ns", "cluster", io)
	g.Expect(err).To(HaveOccurred())
	g.Expect(deleted).To(BeFalse())
	g.Expect(err.Error()).To(ContainSubstring("checking deletion status"))
}

func TestWaitForDeletion_EventualDeletion(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	dynResource := dyn.Resource(resources.RayCluster.GVR())

	callCount := 0
	dyn.PrependReactor("get", "rayclusters", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		callCount++
		if callCount <= 2 {
			return true, &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": resources.RayCluster.APIVersion(),
				"kind":       resources.RayCluster.Kind,
				"metadata":   map[string]any{"name": "cluster", "namespace": "ns"},
			}}, nil
		}

		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Group: "ray.io", Resource: "rayclusters"}, "cluster",
		)
	})

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	deleted, err := waitForDeletion(t.Context(), dynResource, "ns", "cluster", io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deleted).To(BeTrue())
	g.Expect(callCount).To(Equal(3))
}

// --- confirmLiveMigration (non-dryrun, non-skipconfirm paths) ---

func TestConfirmLiveMigration_AllNamespacesWarning(t *testing.T) {
	g := NewWithT(t)

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata":   map[string]any{"name": "test", "namespace": "ns"},
			"spec":       map[string]any{},
		},
	}

	var inBuf bytes.Buffer
	inBuf.WriteString("n\n")
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(&inBuf, nil, &errBuf)

	result := confirmLiveMigration([]*unstructured.Unstructured{rc}, "", "",
		migrateOptions{}, io)
	g.Expect(result).To(BeFalse())
	g.Expect(errBuf.String()).To(ContainSubstring("ALL clusters across ALL namespaces"))
	g.Expect(errBuf.String()).To(ContainSubstring("cancelled"))
}

func TestConfirmLiveMigration_SpecificNamespaceWarning(t *testing.T) {
	g := NewWithT(t)

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata":   map[string]any{"name": "test", "namespace": "ns"},
			"spec":       map[string]any{},
		},
	}

	var inBuf bytes.Buffer
	inBuf.WriteString("n\n")
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(&inBuf, nil, &errBuf)

	result := confirmLiveMigration([]*unstructured.Unstructured{rc}, "", "my-ns",
		migrateOptions{}, io)
	g.Expect(result).To(BeFalse())
	g.Expect(errBuf.String()).To(ContainSubstring("ALL clusters in namespace 'my-ns'"))
}

func TestConfirmLiveMigration_Accept(t *testing.T) {
	g := NewWithT(t)

	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "ray.io/v1",
			"kind":       "RayCluster",
			"metadata":   map[string]any{"name": "test", "namespace": "ns"},
			"spec":       map[string]any{},
		},
	}

	var inBuf bytes.Buffer
	inBuf.WriteString("y\n")
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(&inBuf, nil, &errBuf)

	result := confirmLiveMigration([]*unstructured.Unstructured{rc}, "test", "ns",
		migrateOptions{}, io)
	g.Expect(result).To(BeTrue())
}

// --- confirmBackupRestore (non-dryrun, non-skipconfirm paths) ---

func TestConfirmBackupRestore_Declined(t *testing.T) {
	g := NewWithT(t)

	var inBuf bytes.Buffer
	inBuf.WriteString("n\n")
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(&inBuf, nil, &errBuf)

	result := confirmBackupRestore(migrateOptions{}, io)
	g.Expect(result).To(BeFalse())
	g.Expect(errBuf.String()).To(ContainSubstring("cancelled"))
}

func TestConfirmBackupRestore_Accepted(t *testing.T) {
	g := NewWithT(t)

	var inBuf bytes.Buffer
	inBuf.WriteString("y\n")
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(&inBuf, nil, &errBuf)

	result := confirmBackupRestore(migrateOptions{}, io)
	g.Expect(result).To(BeTrue())
}

// --- reportNoYAMLFiles ---

func TestReportNoYAMLFiles_WithSubdirs(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	g.Expect(os.Mkdir(filepath.Join(dir, "rhoai-3.x"), 0o750)).To(Succeed())
	g.Expect(os.Mkdir(filepath.Join(dir, "rhoai-2.x"), 0o750)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)
	reportNoYAMLFiles(dir, io)
	g.Expect(errBuf.String()).To(ContainSubstring("Found subdirectories"))
	g.Expect(errBuf.String()).To(ContainSubstring("Hint"))
}

func TestReportNoYAMLFiles_FileInput(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	g.Expect(os.WriteFile(f, []byte("test"), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)
	reportNoYAMLFiles(f, io)
	g.Expect(errBuf.String()).ToNot(ContainSubstring("subdirectories"))
}

func TestReportNoYAMLFiles_NonexistentPath(t *testing.T) {
	g := NewWithT(t)
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)
	reportNoYAMLFiles("/nonexistent/path", io)
	g.Expect(errBuf.String()).To(ContainSubstring("No YAML files found"))
}

// --- parseBackupItems ---

func TestParseBackupItems_FiltersCorrectly(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	rc1 := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: cluster-a
  namespace: ns-a
`
	rc2 := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: cluster-b
  namespace: ns-b
`
	g.Expect(os.WriteFile(filepath.Join(dir, "rc1.yaml"), []byte(rc1), 0o600)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(dir, "rc2.yaml"), []byte(rc2), 0o600)).To(Succeed())

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	items := parseBackupItems(
		[]string{filepath.Join(dir, "rc1.yaml"), filepath.Join(dir, "rc2.yaml")},
		"cluster-a", "", io,
	)
	g.Expect(items).To(HaveLen(1))
	g.Expect(items[0].u.GetName()).To(Equal("cluster-a"))
}

// --- getClusterState ---

func newFakeClientInternal(t *testing.T, dynamicClient *dynamicfake.FakeDynamicClient) client.Client {
	t.Helper()

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func TestGetClusterState_Found(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata":   map[string]any{"name": "cluster", "namespace": "ns"},
			"status":     map[string]any{"state": "ready"},
		},
	}
	_, err := dyn.Resource(resources.RayCluster.GVR()).Namespace("ns").Create(t.Context(), rc, metav1.CreateOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	c := newFakeClientInternal(t, dyn)
	state := getClusterState(t.Context(), c, "cluster", "ns")
	g.Expect(state).To(Equal("ready"))
}

func TestGetClusterState_NotFound(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	c := newFakeClientInternal(t, dyn)
	state := getClusterState(t.Context(), c, "nonexistent", "ns")
	g.Expect(state).To(BeEmpty())
}

func TestGetClusterState_NoStatusField(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata":   map[string]any{"name": "cluster", "namespace": "ns"},
			"spec":       map[string]any{},
		},
	}
	_, err := dyn.Resource(resources.RayCluster.GVR()).Namespace("ns").Create(t.Context(), rc, metav1.CreateOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	c := newFakeClientInternal(t, dyn)
	state := getClusterState(t.Context(), c, "cluster", "ns")
	g.Expect(state).To(BeEmpty())
}

// --- waitForClusterReady ---

func TestWaitForClusterReady_ImmediateReady(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	rc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata":   map[string]any{"name": "cluster", "namespace": "ns"},
			"status":     map[string]any{"state": "ready"},
		},
	}
	_, err := dyn.Resource(resources.RayCluster.GVR()).Namespace("ns").Create(t.Context(), rc, metav1.CreateOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	c := newFakeClientInternal(t, dyn)
	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	state := waitForClusterReady(t.Context(), c, "cluster", "ns", 10*time.Second, io)
	g.Expect(state).To(Equal("ready"))
	g.Expect(errBuf.String()).To(ContainSubstring("Waiting for cluster pods to become healthy"))
}

func TestWaitForClusterReady_EventualReady(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	c := newFakeClientInternal(t, dyn)

	callCount := 0
	dyn.PrependReactor("get", "rayclusters", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		callCount++
		state := "suspended"
		if callCount >= 3 {
			state = "ready"
		}

		return true, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata":   map[string]any{"name": "cluster", "namespace": "ns"},
			"status":     map[string]any{"state": state},
		}}, nil
	})

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	state := waitForClusterReady(t.Context(), c, "cluster", "ns", 30*time.Second, io)
	g.Expect(state).To(Equal("ready"))
	g.Expect(callCount).To(BeNumerically(">=", 3))
}

func TestWaitForClusterReady_CancelledContext(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	dyn := newFakeDynamic(t)
	c := newFakeClientInternal(t, dyn)

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	start := time.Now()
	state := waitForClusterReady(ctx, c, "cluster", "ns", 30*time.Second, io)
	elapsed := time.Since(start)

	g.Expect(state).To(BeEmpty())
	g.Expect(elapsed).To(BeNumerically("<", 2*time.Second))
}

func TestWaitForClusterReady_TimeoutReturnsLastState(t *testing.T) {
	g := NewWithT(t)

	dyn := newFakeDynamic(t)
	c := newFakeClientInternal(t, dyn)

	dyn.PrependReactor("get", "rayclusters", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata":   map[string]any{"name": "cluster", "namespace": "ns"},
			"status":     map[string]any{"state": "suspended"},
		}}, nil
	})

	var errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, nil, &errBuf)

	state := waitForClusterReady(t.Context(), c, "cluster", "ns", 1*time.Millisecond, io)
	g.Expect(state).To(Equal("suspended"))
}

// --- clusterStateMessage ---

func TestClusterStateMessage(t *testing.T) {
	tests := []struct {
		state    string
		expected string
	}{
		{"ready", "ready"},
		{"", "not yet ready (timeout)"},
		{"suspended", "suspended (not yet ready)"},
		{"unhealthy", "unhealthy (not yet ready)"},
	}

	for _, tt := range tests {
		t.Run(tt.state+"->"+tt.expected, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(clusterStateMessage(tt.state)).To(Equal(tt.expected))
		})
	}
}

// --- executeLiveMigration ---
