package aipipelines

import (
	"io/fs"
	"time"
)

const (
	preUpgradeCheckID          = "ai-pipelines.pre-upgrade-check"
	preUpgradeCheckName        = "AI Pipelines pre-upgrade check"
	preUpgradeCheckDescription = "Captures DSPA pod health, migrates v1alpha1 resources to v1, and detects RBAC gaps"

	updateDSPRoleID          = "ai-pipelines.update-dsp-role"
	updateDSPRoleName        = "Update custom DSP roles"
	updateDSPRoleDescription = "Patches custom RBAC roles to add datasciencepipelinesapplications/api subresource"

	postUpgradeCheckID          = "ai-pipelines.post-upgrade-check"
	postUpgradeCheckName        = "AI Pipelines post-upgrade check"
	postUpgradeCheckDescription = "Verifies pipeline server pods are healthy post-upgrade against pre-upgrade baseline"
)

const (
	podPrefixDSPipeline = "ds-pipeline-"
	podPrefixMariaDB    = "mariadb-"
)

// Uses /tmp because the CLI runs in OpenShift containers with arbitrary UIDs where
// $HOME (/) and $XDG_CACHE_HOME are not writable. The Dockerfile provisions this
// directory with chmod 1777.
const defaultStateDir = "/tmp/rhoai-upgrade-backup/ai_pipelines"
const stateFileName = "dspa_pre_upgrade_pods.json"

const systemNamespacePattern = `^(kube-system|default|openshift.*|redhat-ods-.*)$`

const (
	retryMaxSteps        = 10
	retryInitialDuration = 1 * time.Second
	retryFactor          = 2.0
	retryJitter          = 0.1
	retryMaxDuration     = 30 * time.Second
)

const (
	postUpgradePollInterval = 15 * time.Second
	postUpgradeTimeout      = 120 * time.Second
)

const dspaAPIGroup = "datasciencepipelinesapplications.opendatahub.io"

const (
	dirPermissions  fs.FileMode = 0o750
	filePermissions fs.FileMode = 0o600
)
