package data

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	podexec "github.com/opendatahub-io/odh-cli/pkg/util/exec"
)

const (
	trustyaiContainer     = "trustyai-service"
	mariadbContainer      = "mariadb"
	storageFormatPVC      = "PVC"
	storageFormatDatabase = "DATABASE"
	metadataFileName      = "metadata.json"
	dumpFileName          = "dump.sql"
	dataSubDir            = "data"
	operatorVolumeName    = "volume"
	backupDirPermission   = 0o755
	backupFilePermission  = 0o644
)

//nolint:gochecknoglobals // Static key lists for credential extraction; Go has no const slices.
var (
	usernameKeys = []string{
		"databaseUsername", "databaseUser", "database-username", "database-user",
		"MYSQL_USER", "DB_USER", "user", "username",
	}
	passwordKeys = []string{
		"databasePassword", "database-password",
		"MYSQL_PASSWORD", "DB_PASSWORD", "password",
	}
	databaseKeys = []string{
		"databaseName", "database-name",
		"MYSQL_DATABASE", "DB_NAME", "database",
	}
	serviceHostKeys = []string{
		"databaseService", "database-service", "DB_HOST",
	}
)

type serviceInfo struct {
	name          string
	namespace     string
	storageFormat string
	cr            *unstructured.Unstructured
}

type mountInfo struct {
	mountPath string
	pvcName   string
}

type dbCredentials struct {
	secretName string
	username   string
	password   string
	database   string
}

// DataBackupMetadata captures all parameters of a data backup for use during restore.
type DataBackupMetadata struct {
	Timestamp         string `json:"timestamp"`
	Namespace         string `json:"namespace"`
	TrustyAIService   string `json:"trustyaiService"`
	StorageFormat     string `json:"storageFormat"`
	PVCName           string `json:"pvcName,omitempty"`
	MountPath         string `json:"mountPath,omitempty"`
	SourcePod         string `json:"sourcePod,omitempty"`
	FileCount         int    `json:"fileCount,omitempty"`
	MariaDBPod        string `json:"mariadbPod,omitempty"`
	CredentialsSecret string `json:"credentialsSecret,omitempty"`
	DatabaseName      string `json:"databaseName,omitempty"`
	DatabaseUser      string `json:"databaseUser,omitempty"`
	DumpCommand       string `json:"dumpCommand,omitempty"`
	DumpLines         int    `json:"dumpLines,omitempty"`
}

func discoverTrustyAIServices(ctx context.Context, target action.Target, serviceName string) ([]serviceInfo, error) {
	if serviceName != "" {
		services, err := target.Client.List(ctx, resources.TrustyAIService)
		if err != nil {
			return nil, fmt.Errorf("listing TrustyAIService resources: %w", err)
		}

		var filtered []serviceInfo

		for _, svc := range services {
			if svc.GetName() == serviceName {
				format, _, _ := unstructured.NestedString(svc.Object, "spec", "storage", "format")
				filtered = append(filtered, serviceInfo{
					name:          svc.GetName(),
					namespace:     svc.GetNamespace(),
					storageFormat: normalizeStorageFormat(format),
					cr:            svc,
				})
			}
		}

		return filtered, nil
	}

	services, err := target.Client.List(ctx, resources.TrustyAIService)
	if err != nil {
		return nil, fmt.Errorf("listing TrustyAIService resources: %w", err)
	}

	var result []serviceInfo

	for _, svc := range services {
		format, _, _ := unstructured.NestedString(svc.Object, "spec", "storage", "format")
		result = append(result, serviceInfo{
			name:          svc.GetName(),
			namespace:     svc.GetNamespace(),
			storageFormat: normalizeStorageFormat(format),
			cr:            svc,
		})
	}

	return result, nil
}

func normalizeStorageFormat(format string) string {
	switch strings.ToUpper(format) {
	case storageFormatPVC, "":
		return storageFormatPVC
	case storageFormatDatabase, "DB":
		return storageFormatDatabase
	default:
		return format
	}
}

func findServicePod(ctx context.Context, target action.Target, namespace, serviceName string) (*corev1.Pod, error) {
	labels := []string{
		"app=" + serviceName,
		"app.kubernetes.io/name=" + serviceName,
		"app.kubernetes.io/part-of=trustyai",
	}

	for _, label := range labels {
		pod, err := findRunningPodByLabel(ctx, target, namespace, label)
		if err != nil {
			return nil, err
		}

		if pod != nil {
			return pod, nil
		}
	}

	pod, err := findRunningPodByName(ctx, target, namespace, serviceName)
	if err != nil {
		return nil, err
	}

	if pod != nil {
		return pod, nil
	}

	return nil, fmt.Errorf("no running TrustyAI pod found in namespace %q for service %q", namespace, serviceName)
}

func findMariaDBPod(ctx context.Context, target action.Target, namespace, serviceName string, secretData map[string][]byte) (*corev1.Pod, error) {
	patterns := []string{
		"mariadb-" + serviceName,
		"mariadb",
		"mysql",
	}

	for _, pattern := range patterns {
		pod, err := findRunningPodByName(ctx, target, namespace, pattern)
		if err != nil {
			return nil, err
		}

		if pod != nil {
			return pod, nil
		}
	}

	svcHost := trySecretKeys(secretData, serviceHostKeys)
	if svcHost != "" {
		pod, err := findRunningPodByName(ctx, target, namespace, svcHost)
		if err != nil {
			return nil, err
		}

		if pod != nil {
			return pod, nil
		}
	}

	return nil, fmt.Errorf("no running MariaDB pod found in namespace %q", namespace)
}

func findRunningPodByLabel(ctx context.Context, target action.Target, namespace, label string) (*corev1.Pod, error) {
	pods, err := target.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return nil, fmt.Errorf("listing pods with label %q: %w", label, err)
	}

	if len(pods.Items) == 0 {
		return nil, nil
	}

	return &pods.Items[0], nil
}

func findRunningPodByName(ctx context.Context, target action.Target, namespace, namePattern string) (*corev1.Pod, error) {
	pods, err := target.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return nil, fmt.Errorf("listing running pods: %w", err)
	}

	lowerPattern := strings.ToLower(namePattern)

	for i := range pods.Items {
		if strings.Contains(strings.ToLower(pods.Items[i].Name), lowerPattern) {
			return &pods.Items[i], nil
		}
	}

	return nil, nil
}

func findMountPath(pod *corev1.Pod, cr *unstructured.Unstructured, serviceName string) (*mountInfo, error) {
	if info := findMountByVolumeName(pod, operatorVolumeName); info != nil {
		return info, nil
	}

	if info := findMountByPVCName(pod, serviceName); info != nil {
		return info, nil
	}

	if cr != nil {
		folder, found, _ := unstructured.NestedString(cr.Object, "spec", "storage", "folder")
		if found && folder != "" {
			return &mountInfo{
				mountPath: folder,
				pvcName:   serviceName + "-pvc",
			}, nil
		}
	}

	return nil, fmt.Errorf("could not determine PVC mount path for pod %q", pod.Name)
}

func findMountByVolumeName(pod *corev1.Pod, volumeName string) *mountInfo { //nolint:unparam // volumeName is parameterised for testability
	var pvcName string

	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == volumeName && pod.Spec.Volumes[i].PersistentVolumeClaim != nil {
			pvcName = pod.Spec.Volumes[i].PersistentVolumeClaim.ClaimName

			break
		}
	}

	for i := range pod.Spec.Containers {
		for j := range pod.Spec.Containers[i].VolumeMounts {
			if pod.Spec.Containers[i].VolumeMounts[j].Name == volumeName {
				return &mountInfo{
					mountPath: pod.Spec.Containers[i].VolumeMounts[j].MountPath,
					pvcName:   pvcName,
				}
			}
		}
	}

	return nil
}

func findMountByPVCName(pod *corev1.Pod, serviceName string) *mountInfo {
	lowerName := strings.ToLower(serviceName)

	for i := range pod.Spec.Volumes {
		vol := &pod.Spec.Volumes[i]
		if vol.PersistentVolumeClaim == nil {
			continue
		}

		if !strings.Contains(strings.ToLower(vol.PersistentVolumeClaim.ClaimName), lowerName) {
			continue
		}

		for j := range pod.Spec.Containers {
			for k := range pod.Spec.Containers[j].VolumeMounts {
				if pod.Spec.Containers[j].VolumeMounts[k].Name == vol.Name {
					return &mountInfo{
						mountPath: pod.Spec.Containers[j].VolumeMounts[k].MountPath,
						pvcName:   vol.PersistentVolumeClaim.ClaimName,
					}
				}
			}
		}
	}

	return nil
}

func findDBCredentials(ctx context.Context, target action.Target, namespace string, cr *unstructured.Unstructured, serviceName string) (*dbCredentials, error) {
	secretName, err := findCredentialsSecret(ctx, target, namespace, cr, serviceName)
	if err != nil {
		return nil, err
	}

	secretObj, err := target.Client.GetResource(ctx, resources.Secret, secretName,
		client.InNamespace(namespace))
	if err != nil {
		return nil, fmt.Errorf("getting secret %q: %w", secretName, err)
	}

	secretData := extractSecretData(secretObj)

	username := trySecretKeys(secretData, usernameKeys)
	if username == "" {
		return nil, fmt.Errorf("could not find username in secret %q", secretName)
	}

	password := trySecretKeys(secretData, passwordKeys)
	if password == "" {
		return nil, fmt.Errorf("could not find password in secret %q", secretName)
	}

	database := trySecretKeys(secretData, databaseKeys)
	if database == "" {
		return nil, fmt.Errorf("could not find database name in secret %q", secretName)
	}

	return &dbCredentials{
		secretName: secretName,
		username:   username,
		password:   password,
		database:   database,
	}, nil
}

func findCredentialsSecret(ctx context.Context, target action.Target, namespace string, cr *unstructured.Unstructured, serviceName string) (string, error) {
	if cr != nil {
		name, found, _ := unstructured.NestedString(cr.Object, "spec", "storage", "databaseConfigurations")
		if found && name != "" {
			if secretExists(ctx, target, namespace, name) {
				return name, nil
			}
		}
	}

	conventionName := serviceName + "-db-credentials"
	if secretExists(ctx, target, namespace, conventionName) {
		return conventionName, nil
	}

	name, err := findSecretByPattern(ctx, target, namespace, "db-credentials")
	if err != nil {
		return "", err
	}

	if name != "" {
		return name, nil
	}

	name, err = findSecretByPattern(ctx, target, namespace, "mariadb")
	if err != nil {
		return "", err
	}

	if name != "" {
		return name, nil
	}

	return "", fmt.Errorf("no database credentials secret found in namespace %q", namespace)
}

func secretExists(ctx context.Context, target action.Target, namespace, name string) bool {
	_, err := target.Client.GetResource(ctx, resources.Secret, name, client.InNamespace(namespace))

	return err == nil
}

func findSecretByPattern(ctx context.Context, target action.Target, namespace, pattern string) (string, error) {
	secrets, err := target.Client.List(ctx, resources.Secret, client.WithNamespace(namespace))
	if err != nil {
		return "", fmt.Errorf("listing secrets: %w", err)
	}

	lowerPattern := strings.ToLower(pattern)

	for _, s := range secrets {
		if strings.Contains(strings.ToLower(s.GetName()), lowerPattern) {
			return s.GetName(), nil
		}
	}

	return "", nil
}

func extractSecretData(secret *unstructured.Unstructured) map[string][]byte {
	dataField, found, _ := unstructured.NestedMap(secret.Object, "data")
	if !found {
		return nil
	}

	result := make(map[string][]byte, len(dataField))

	for k, v := range dataField {
		strVal, ok := v.(string)
		if !ok {
			continue
		}

		decoded, err := base64.StdEncoding.DecodeString(strVal)
		if err != nil {
			continue
		}

		result[k] = decoded
	}

	return result
}

func trySecretKeys(data map[string][]byte, keys []string) string {
	for _, key := range keys {
		if val, ok := data[key]; ok && len(val) > 0 {
			return strings.TrimSpace(string(val))
		}
	}

	return ""
}

func detectDumpCommand(ctx context.Context, executor podexec.Executor, namespace string, pod *corev1.Pod) (string, error) {
	commands := []string{"mariadb-dump", "mysqldump"}

	for _, name := range commands {
		if commandExistsInPod(ctx, executor, namespace, pod, name) {
			return name, nil
		}
	}

	return "", fmt.Errorf("neither mariadb-dump nor mysqldump found in pod %q", pod.Name)
}

func detectClientCommand(ctx context.Context, executor podexec.Executor, namespace string, pod *corev1.Pod) (string, error) {
	commands := []string{"mariadb", "mysql"}

	for _, name := range commands {
		if commandExistsInPod(ctx, executor, namespace, pod, name) {
			return name, nil
		}
	}

	return "", fmt.Errorf("neither mariadb nor mysql client found in pod %q", pod.Name)
}

func commandExistsInPod(ctx context.Context, executor podexec.Executor, namespace string, pod *corev1.Pod, command string) bool {
	var stderr bytes.Buffer

	err := executor.Exec(ctx, podexec.ExecOptions{
		Namespace:     namespace,
		PodName:       pod.Name,
		ContainerName: mariadbContainer,
		Command:       []string{"which", command},
		Stdout:        io.Discard,
		Stderr:        &stderr,
	})

	return err == nil
}

func writeMetadata(backupPath string, metadata DataBackupMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling metadata: %w", err)
	}

	metaPath := filepath.Join(backupPath, metadataFileName)

	if err := os.WriteFile(metaPath, data, backupFilePermission); err != nil {
		return fmt.Errorf("writing metadata file: %w", err)
	}

	return nil
}

func countFiles(dir string) (int, error) {
	count := 0

	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			count++
		}

		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("walking directory: %w", err)
	}

	return count, nil
}

func countLines(filePath string) (int, error) {
	f, err := os.Open(filePath) //nolint:gosec // Path from controlled backup directory.
	if err != nil {
		return 0, fmt.Errorf("opening file: %w", err)
	}
	defer func() { _ = f.Close() }()

	count := 0

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			count++
		}
		if err != nil {
			if err == io.EOF {
				break
			}

			return 0, fmt.Errorf("reading file: %w", err)
		}
	}

	return count, nil
}

func detectBackupType(backupFile string) (string, error) {
	metadataPath := filepath.Join(backupFile, metadataFileName)
	if data, err := os.ReadFile(metadataPath); err == nil { //nolint:gosec // Path from validated backup dir.
		var meta DataBackupMetadata
		if jsonErr := json.Unmarshal(data, &meta); jsonErr == nil && meta.StorageFormat != "" {
			return normalizeStorageFormat(meta.StorageFormat), nil
		}
	}

	dataDir := filepath.Join(backupFile, dataSubDir)
	if info, err := os.Stat(dataDir); err == nil && info.IsDir() {
		return storageFormatPVC, nil
	}

	dumpFile := filepath.Join(backupFile, dumpFileName)
	if _, err := os.Stat(dumpFile); err == nil {
		return storageFormatDatabase, nil
	}

	return "", fmt.Errorf("cannot determine backup type from %q (expected data/ directory or dump.sql file)", backupFile)
}

func loadMetadata(backupFile string) *DataBackupMetadata {
	metadataPath := filepath.Join(backupFile, metadataFileName)

	data, err := os.ReadFile(metadataPath) //nolint:gosec // Path from validated backup dir.
	if err != nil {
		return nil
	}

	var meta DataBackupMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}

	return &meta
}

func containsSQLError(output string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "ERROR") {
			return true
		}
	}

	return false
}
