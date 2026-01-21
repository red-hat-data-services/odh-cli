# Writing Lint Checks

**Scope:** This guide explains how to write diagnostic checks for the **lint command**.

The lint command (`kubectl odh lint`) validates OpenShift AI cluster configuration using a check framework. This guide covers how to implement checks specifically for the lint command.

For lint architecture, see [architecture.md](architecture.md). For general development practices, see [../development.md](../development.md).

## Check Interface

All lint checks implement the `Check` interface:

```go
type Check interface {
    ID() string
    Name() string
    Description() string
    Group() string
    CanApply(ctx context.Context, target *CheckTarget) bool
    Validate(ctx context.Context, target *CheckTarget) *DiagnosticResult
}
```

### Implementing a Lint Check

Create a new package under `pkg/lint/checks/<category>/<checkname>/`:

```go
// pkg/lint/checks/components/dashboard/dashboard.go
package dashboard

import (
    "context"
    "github.com/blang/semver/v4"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
)

type Check struct {
    base.BaseCheck
}

func NewCheck() *Check {
    return &Check{
        BaseCheck: base.BaseCheck{
            CheckGroup:       check.GroupComponent,
            Kind:             check.ComponentDashboard,
            CheckType:        check.CheckTypeInstalled,
            CheckID:          "components.dashboard.status",
            CheckName:        "Components :: Dashboard :: Status",
            CheckDescription: "Validates dashboard component configuration and availability",
        },
    }
}

func (c *Check) CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool {
    // Check applies to all versions
    return true
}

func (c *Check) Validate(ctx context.Context, target *check.CheckTarget) (*result.DiagnosticResult, error) {
    dr := c.NewResult()
    // Implementation
    return dr, nil
}

func init() {
    check.MustRegisterCheck(NewCheck())
}
```

### Using BaseCheck

**BaseCheck** eliminates boilerplate by providing common check metadata through composition:

**Benefits:**
- No need to define constants for ID, name, description
- No need to implement `ID()`, `Name()`, `Description()`, `Group()` methods
- `NewResult()` automatically creates results with check metadata
- Public fields `Kind` and `CheckType` can be accessed directly
- ~35% code reduction per check

**When to use:**
- All new checks should use BaseCheck
- Access metadata via public fields: `c.Kind`, `c.CheckType`, `c.CheckGroup`, etc.

## Registration Pattern

Lint checks self-register using `init()` functions:

```go
func init() {
    registry.Instance().Register(&Check{})
}
```

Import the check package with a blank import in the lint command entrypoint:

```go
// cmd/lint/lint.go
import (
    // Import check packages to trigger init() auto-registration.
    // These blank imports are REQUIRED for checks to register with the global registry.
    // Do NOT remove these imports - they appear unused but are essential for runtime check discovery.
    _ "github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/dashboard"
    _ "github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/kserve"
)
```

**Critical:** Always include explanatory comments with blank imports to prevent accidental removal.

## CanApply Versioning Logic

The `CanApply` method determines if a lint check is applicable based on version context.

### Lint Mode vs Upgrade Mode

Lint checks detect their execution mode by comparing versions:

```go
func (c *Check) CanApply(ctx context.Context, target *check.CheckTarget) bool {
    currentVer := target.CurrentVersion.Version
    targetVer := target.Version.Version

    // Lint mode: validating current cluster state
    isLintMode := currentVer == targetVer

    // Upgrade mode: assessing upgrade readiness
    isUpgradeMode := currentVer != targetVer

    // Example: check only applies when upgrading to 3.x
    if isUpgradeMode && targetVer.Major() == 3 {
        return true
    }

    return false
}
```

### Common Patterns

**Check applies to all versions:**
```go
func (c *Check) CanApply(ctx context.Context, target *check.CheckTarget) bool {
    return true
}
```

**Check applies only in upgrade mode:**
```go
func (c *Check) CanApply(ctx context.Context, target *check.CheckTarget) bool {
    return target.CurrentVersion.Version != target.Version.Version
}
```

**Check applies when upgrading to specific version:**
```go
func (c *Check) CanApply(ctx context.Context, target *check.CheckTarget) bool {
    if target.CurrentVersion.Version == target.Version.Version {
        return false // Not upgrade mode
    }
    return target.Version.Version.Major() == 3
}
```

## DiagnosticResult Construction

### Creating Results

Use the `New()` constructor:

```go
result := check.NewDiagnosticResult(
    "components",           // Group
    "dashboard",           // Kind
    "component-status",    // Name
    "Validates dashboard component configuration and availability", // Description
)
```

### Creating Conditions

Use `check.NewCondition()` to create conditions with automatic Impact derivation:

```go
import (
    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Success: Status=True → Impact=None (auto-derived)
condition := check.NewCondition(
    check.ConditionTypeAvailable,
    metav1.ConditionTrue,
    check.ReasonResourceAvailable,
    "Dashboard component is ready",
)

// Failure: Status=False → Impact=Blocking (auto-derived)
condition := check.NewCondition(
    check.ConditionTypeConfigured,
    metav1.ConditionFalse,
    check.ReasonConfigurationInvalid,
    "Required configuration parameter 'replicas' not set",
)

// Override impact: Status=False but non-blocking
condition := check.NewCondition(
    check.ConditionTypeCompatible,
    metav1.ConditionFalse,
    check.ReasonDeprecated,
    "TrainingOperator is deprecated in RHOAI 3.3",
    check.WithImpact(result.ImpactAdvisory),  // Override to advisory
)

// Printf-style formatting
condition := check.NewCondition(
    check.ConditionTypeCompatible,
    metav1.ConditionFalse,
    check.ReasonWorkloadsImpacted,
    "Found %d active PyTorchJobs - workloads use deprecated TrainingOperator",
    activeCount,
    check.WithImpact(result.ImpactAdvisory),
)
```

**Impact Auto-Derivation Rules:**
- Status=True → Impact=None (requirement met, no issues)
- Status=False → Impact=Blocking (requirement not met, blocks upgrade)
- Status=Unknown → Impact=Advisory (unable to determine, proceed with caution)

**Overriding Impact:**
Use `check.WithImpact()` functional option when the default impact is not appropriate:
- Deprecation warnings (Status=False but non-blocking)
- Advisory notices (Status=False but upgrade can proceed)

**Validation:**
Conditions are validated at creation time. Invalid combinations will panic:
- Status=True with Impact≠None (invalid - if met, there's no impact)
- Status=False/Unknown with Impact=None (invalid - if not met, there must be impact)

### Condition Status and Impact Semantics

**Status** indicates whether a requirement is met:
- **True**: Requirement is MET (check passing)
- **False**: Requirement is NOT MET (check failing)
- **Unknown**: Unable to determine if requirement is met (error state)

**Impact** indicates the upgrade/operational impact:
- **blocking**: Upgrade cannot proceed (critical issue requiring action)
- **advisory**: Upgrade can proceed with warning (non-critical, user should be aware)
- **none** (empty): No impact (success state, requirement met)

**Valid Combinations:**
- Status=True + Impact=None ✓ (requirement met, no issues)
- Status=False + Impact=Blocking ✓ (requirement not met, critical)
- Status=False + Impact=Advisory ✓ (requirement not met, but non-blocking)
- Status=Unknown + Impact=Advisory ✓ (cannot determine, proceed with caution)

**Invalid Combinations (will panic):**
- Status=True + Impact=Blocking ✗ (if requirement is met, there's no blocking impact)
- Status=False + Impact=None ✗ (if requirement is not met, there must be some impact)

### Adding Annotations

Version information is added via annotations:

```go
result.AddAnnotation(
    "check.opendatahub.io/source-version",
    target.CurrentVersion.Version.String(),
)

result.AddAnnotation(
    "check.opendatahub.io/target-version",
    target.Version.Version.String(),
)
```

**Important:** Annotation keys must use domain-qualified format (`domain/key`).

### Validation

Results are automatically validated before being returned. Validation ensures:
- `Metadata.Group` is not empty
- `Metadata.Kind` is not empty
- `Metadata.Name` is not empty
- `Status.Conditions` contains at least one condition
- All condition `Type` fields are not empty
- All condition `Status` values are "True", "False", or "Unknown"
- All condition `Reason` fields are not empty
- All annotation keys are in `domain/key` format

## JQ-Based Field Access

All operations on unstructured objects in lint checks MUST use JQ queries via `pkg/util/jq`.

### Reading Fields

```go
import "github.com/lburgazzoli/odh-cli/pkg/util/jq"

// Read a field value
result, err := jq.Query(obj, ".spec.components.dashboard.managementState")
if err != nil {
    return fmt.Errorf("failed to query management state: %w", err)
}

managementState, ok := result.(string)
if !ok {
    return fmt.Errorf("management state is not a string")
}
```

### Complex Queries

JQ supports full query syntax:

```go
// Query with default value
result, err := jq.Query(obj, ".metadata.labels.version // \"unknown\"")

// Check if field exists
result, err := jq.Query(obj, ".spec.components | has(\"kserve\")")

// Array operations
result, err := jq.Query(obj, ".status.conditions | map(select(.type == \"Ready\")) | length")
```

### Prohibited

Direct use of `unstructured.Unstructured` accessor methods is **prohibited**:
- ❌ `unstructured.NestedString()`
- ❌ `unstructured.NestedField()`
- ❌ `unstructured.SetNestedField()`
- ❌ `unstructured.NestedFieldCopy()`

**Use JQ instead** for consistent, expressive queries.

## Centralized GVK/GVR Usage

All GroupVersionKind (GVK) and GroupVersionResource (GVR) references in lint checks MUST use definitions from `pkg/resources/types.go`.

### Accessing Resource Types

```go
import "github.com/lburgazzoli/odh-cli/pkg/resources"

// Get GVK
gvk := resources.DataScienceCluster.GVK()

// Get GVR for dynamic client
gvr := resources.DataScienceCluster.GVR()

// Get APIVersion for unstructured objects
apiVersion := resources.DataScienceCluster.APIVersion()
```

### Using in Lint Checks

```go
// List DataScienceCluster resources
dscList := &unstructured.UnstructuredList{}
dscList.SetGroupVersionKind(resources.DataScienceCluster.GVK())

err := target.Client.List(ctx, dscList)
```

### Prohibited

Direct construction of GVK/GVR structs is **prohibited**:

```go
// ❌ WRONG: hardcoded GVK
gvk := schema.GroupVersionKind{
    Group:   "datasciencecluster.opendatahub.io",
    Version: "v1",
    Kind:    "DataScienceCluster",
}

// ✓ CORRECT: use centralized definition
gvk := resources.DataScienceCluster.GVK()
```

## High-Level Resource Targeting

Lint checks MUST operate on high-level custom resources, not low-level Kubernetes primitives.

### Permitted Targets

- **Component CRs**: DataScienceCluster, DSCInitialization
- **Workload CRs**: Notebook, InferenceService, RayCluster, PyTorchJob, etc.
- **Service CRs**: Custom resources representing OpenShift AI services
- **CRDs**: CustomResourceDefinition (for validating CRD presence)
- **OLM resources**: ClusterServiceVersion, Subscription

### Prohibited Targets

Lint checks MUST NOT target these as primary resources:
- ❌ Pod, Deployment, StatefulSet, ReplicaSet, DaemonSet
- ❌ Service, Ingress, Route
- ❌ ConfigMap, Secret
- ❌ PersistentVolume, PersistentVolumeClaim

**Exception:** Low-level resources MAY be queried as supporting evidence during high-level CR validation (e.g., checking if a Dashboard CR's backing Deployment exists), but MUST NOT be the primary target of a lint check.

### Example

```go
// ✓ CORRECT: Check Dashboard CR
func (c *Check) Validate(ctx context.Context, target *check.CheckTarget) *check.DiagnosticResult {
    dsc := &unstructured.Unstructured{}
    dsc.SetGroupVersionKind(resources.DataScienceCluster.GVK())

    err := target.Client.Get(ctx, client.ObjectKey{Name: "default"}, dsc)
    // Validate Dashboard component within DSC
}

// ❌ WRONG: Check Dashboard Deployment directly
func (c *Check) Validate(ctx context.Context, target *check.CheckTarget) *check.DiagnosticResult {
    deployment := &appsv1.Deployment{}
    err := target.Client.Get(ctx, client.ObjectKey{
        Namespace: "opendatahub",
        Name:      "odh-dashboard",
    }, deployment)
    // This violates high-level resource principle
}
```

## Cluster-Wide Scope

Lint checks MUST operate cluster-wide and scan all namespaces. Namespace filtering is prohibited.

### Discovering Resources

When discovering workloads or services, scan all namespaces:

```go
// ✓ CORRECT: List across all namespaces
notebooks := &unstructured.UnstructuredList{}
notebooks.SetGroupVersionKind(resources.Notebook.GVK())

err := target.Client.List(ctx, notebooks)  // No namespace restriction

// ❌ WRONG: Restrict to specific namespace
err := target.Client.List(ctx, notebooks, &client.ListOptions{
    Namespace: "opendatahub",  // Prohibited
})
```

### Handling Multi-Namespace Results

Process resources from all namespaces:

```go
for _, item := range notebooks.Items {
    namespace, _ := jq.Query(&item, ".metadata.namespace")
    name, _ := jq.Query(&item, ".metadata.name")

    // Validate notebook configuration
    // Report issues with namespace context in condition message
}
```

## Complete Example

Here's a complete lint check implementation using BaseCheck and the new Impact-based API:

```go
// pkg/lint/checks/components/kserve/deprecation.go
package kserve

import (
    "context"
    "fmt"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
    "github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
    "github.com/lburgazzoli/odh-cli/pkg/util/jq"
    "github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type DeprecationCheck struct {
    base.BaseCheck
}

func NewDeprecationCheck() *DeprecationCheck {
    return &DeprecationCheck{
        BaseCheck: base.BaseCheck{
            CheckGroup:       check.GroupComponent,
            Kind:             check.ComponentKServe,
            CheckType:        check.CheckTypeDeprecation,
            CheckID:          "components.kserve.deprecation",
            CheckName:        "Components :: KServe :: Serverless Deprecation",
            CheckDescription: "Validates that serverless components are removed when upgrading to 3.x",
        },
    }
}

func (c *DeprecationCheck) CanApply(target check.Target) bool {
    // Only applies when upgrading to 3.x
    //nolint:mnd // Version numbers 3.0
    return version.IsVersionAtLeast(target.TargetVersion, 3, 0)
}

func (c *DeprecationCheck) Validate(
    ctx context.Context,
    target check.Target,
) (*result.DiagnosticResult, error) {
    dr := c.NewResult()

    // Get DataScienceCluster
    dsc, err := target.Client.GetDataScienceCluster(ctx)
    switch {
    case apierrors.IsNotFound(err):
        return results.DataScienceClusterNotFound(
            string(c.Group()),
            c.Kind,
            c.CheckType,
            c.Description(),
        ), nil
    case err != nil:
        return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
    }

    // Check serverless configuration using JQ
    serverlessState, err := jq.Query[string](dsc, ".spec.components.kserve.serving.managementState")
    if err != nil {
        return nil, fmt.Errorf("querying serverless managementState: %w", err)
    }

    // Add component state annotation
    dr.Annotations[check.AnnotationComponentManagementState] = serverlessState
    if target.TargetVersion != nil {
        dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
    }

    // Evaluate serverless state
    if serverlessState == "" || serverlessState == check.ManagementStateRemoved {
        // Success: serverless removed or not configured
        results.SetCompatibilitySuccessf(dr,
            "Serverless components are removed (state: %s) - ready for 3.x upgrade",
            serverlessState,
        )
        return dr, nil
    }

    // Failure: serverless still configured (blocking upgrade)
    results.SetCondition(dr, check.NewCondition(
        check.ConditionTypeCompatible,
        metav1.ConditionFalse,
        check.ReasonVersionIncompatible,
        "Serverless components are still configured (state: %s) - must be removed before upgrading to 3.x",
        serverlessState,
        // Default Impact=Blocking (auto-derived from Status=False)
    ))

    return dr, nil
}

//nolint:gochecknoinits
func init() {
    check.MustRegisterCheck(NewDeprecationCheck())
}
```

## Testing Lint Checks

Write tests using vanilla Gomega and fake clients:

```go
// pkg/lint/checks/components/kserve/serverless_test.go
package kserve

import (
    "context"
    "testing"

    . "github.com/onsi/gomega"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"

    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/resources"
)

func TestServerlessRemovalCheck(t *testing.T) {
    g := NewWithT(t)

    t.Run("should pass when serverless is removed", func(t *testing.T) {
        dsc := &unstructured.Unstructured{}
        dsc.SetGroupVersionKind(resources.DataScienceCluster.GVK())
        dsc.SetName("default")
        dsc.Object["spec"] = map[string]any{
            "components": map[string]any{
                "kserve": map[string]any{
                    "serving": map[string]any{
                        "managementState": "Removed",
                    },
                },
            },
        }

        fakeClient := fake.NewClientBuilder().WithObjects(dsc).Build()

        target := &check.CheckTarget{
            Client:         fakeClient,
            CurrentVersion: &version.ClusterVersion{Version: semver.MustParse("2.17.0")},
            Version:        &version.ClusterVersion{Version: semver.MustParse("3.0.0")},
        }

        check := &Check{}
        result := check.Validate(t.Context(), target)

        g.Expect(result).To(HaveField("Metadata.Group", "components"))
        g.Expect(result).To(HaveField("Metadata.Kind", "kserve"))
        g.Expect(result.Status.Conditions).To(HaveLen(1))
        g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
            "Type":   Equal("ServerlessRemoved"),
            "Status": Equal(metav1.ConditionTrue),
            "Reason": Equal("ServerlessRemoved"),
        }))
    })
}
```

See [../testing.md](../testing.md) for comprehensive testing guidelines.
