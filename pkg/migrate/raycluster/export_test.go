package raycluster

import "github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

// ReportNoYAMLFilesForTesting exposes reportNoYAMLFiles for external tests.
func ReportNoYAMLFilesForTesting(backupPath string, io iostreams.Interface) {
	reportNoYAMLFiles(backupPath, io)
}

// ParseBackupItemsForTesting exposes parseBackupItems for external tests.
func ParseBackupItemsForTesting(yamlFiles []string, clusterName, namespace string, io iostreams.Interface) []BackupItemForTesting {
	items := parseBackupItems(yamlFiles, clusterName, namespace, io)
	result := make([]BackupItemForTesting, len(items))
	for i, item := range items {
		result[i] = BackupItemForTesting{Name: item.u.GetName(), File: item.file}
	}

	return result
}

// BackupItemForTesting is a test-visible representation of backupItem.
type BackupItemForTesting struct {
	Name string
	File string
}
