package rhbok

import (
	"context"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/rbac"
)

//nolint:gochecknoglobals // export_test.go exposes internals for external test package
var (
	ExportCheckCurrentKueueState = (*RHBOKMigrationAction).checkCurrentKueueState
	ExportCheckNoRHBOKConflicts  = (*RHBOKMigrationAction).checkNoRHBOKConflicts
	ExportVerifyKueueResources   = (*RHBOKMigrationAction).verifyKueueResources
	ExportCheckKueueManaged      = (*RHBOKMigrationAction).checkKueueManaged
	ExportPreserveKueueConfig    = (*RHBOKMigrationAction).preserveKueueConfig
	ExportInstallRHBOKOperator   = (*RHBOKMigrationAction).installRHBOKOperator
	ExportUpdateDSC              = (*RHBOKMigrationAction).updateDataScienceCluster
	ExportVerifyResources        = (*RHBOKMigrationAction).verifyResourcesPreserved
	ExportPreparePermissions     = preparePermissions
	ExportRunPermissions         = runPermissions

	ExportConfigMapName         = configMapName
	ExportApplicationsNamespace = applicationsNamespace
	ExportOperatorNamespace     = operatorNamespace
	ExportSubscriptionName      = subscriptionName
	ExportCSVNamePrefix         = csvNamePrefix
)

func ExportVerifyRBAC(a *RHBOKMigrationAction, ctx context.Context, target action.Target, checks []rbac.PermissionCheck) {
	a.verifyRBAC(ctx, target, checks)
}
