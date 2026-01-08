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

### Adding Conditions

Each condition represents a specific validation requirement:

```go
result.AddCondition(
    "ComponentReady",                    // Type
    metav1.ConditionTrue,               // Status (True=passing, False=failing)
    "DashboardReady",                   // Reason
    "Dashboard component is ready",     // Message
)

result.AddCondition(
    "ConfigurationValid",
    metav1.ConditionFalse,
    "MissingConfig",
    "Required configuration parameter 'replicas' not set",
)
```

### Condition Status Semantics

- **True**: Requirement is MET (check passing)
- **False**: Requirement is NOT MET (check failing)
- **Unknown**: Unable to determine if requirement is met (error state)

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

Here's a complete lint check implementation:

```go
// pkg/lint/checks/components/kserve/serverless.go
package kserve

import (
    "context"
    "fmt"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check"
    "github.com/lburgazzoli/odh-cli/pkg/lint/check/registry"
    "github.com/lburgazzoli/odh-cli/pkg/resources"
    "github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

const (
    checkID          = "components.kserve.serverless-removal"
    checkName        = "KServe Serverless Removal"
    checkDescription = "Validates that serverless components are removed when upgrading to 3.x"
)

func init() {
    registry.Instance().Register(&Check{})
}

type Check struct{}

func (c *Check) ID() string          { return checkID }
func (c *Check) Name() string        { return checkName }
func (c *Check) Description() string { return checkDescription }
func (c *Check) Group() string       { return "components" }

func (c *Check) CanApply(ctx context.Context, target *check.CheckTarget) bool {
    // Only applies when upgrading to 3.x
    if target.CurrentVersion.Version == target.Version.Version {
        return false
    }
    return target.Version.Version.Major() == 3
}

func (c *Check) Validate(ctx context.Context, target *check.CheckTarget) *check.DiagnosticResult {
    result := check.NewDiagnosticResult(
        "components",
        "kserve",
        "serverless-removal",
        "Validates that serverless components are removed when upgrading to 3.x",
    )

    // Add version annotations
    result.AddAnnotation(
        "check.opendatahub.io/source-version",
        target.CurrentVersion.Version.String(),
    )
    result.AddAnnotation(
        "check.opendatahub.io/target-version",
        target.Version.Version.String(),
    )

    // Get DataScienceCluster
    dsc := &unstructured.Unstructured{}
    dsc.SetGroupVersionKind(resources.DataScienceCluster.GVK())

    err := target.Client.Get(ctx, client.ObjectKey{Name: "default"}, dsc)
    if err != nil {
        result.AddCondition(
            "ServerlessRemoved",
            metav1.ConditionUnknown,
            "DSCNotFound",
            fmt.Sprintf("Unable to retrieve DataScienceCluster: %v", err),
        )
        return result
    }

    // Check if serverless is configured (using JQ)
    serverlessState, err := jq.Query(dsc, ".spec.components.kserve.serving.managementState")
    if err != nil || serverlessState == nil {
        // Serverless not configured - condition met
        result.AddCondition(
            "ServerlessRemoved",
            metav1.ConditionTrue,
            "ServerlessNotConfigured",
            "Serverless components are not configured",
        )
        return result
    }

    state, ok := serverlessState.(string)
    if !ok || state == "Removed" {
        result.AddCondition(
            "ServerlessRemoved",
            metav1.ConditionTrue,
            "ServerlessRemoved",
            "Serverless components are removed",
        )
    } else {
        result.AddCondition(
            "ServerlessRemoved",
            metav1.ConditionFalse,
            "ServerlessStillConfigured",
            fmt.Sprintf("Serverless managementState is %q (expected Removed)", state),
        )
    }

    return result
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

See [../development.md](../development.md#testing-guidelines) for comprehensive testing guidelines.
