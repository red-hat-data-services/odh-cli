package raycluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// PreflightCheck represents a single pre-upgrade check result.
type PreflightCheck struct {
	Name     string
	Passed   bool
	Message  string
	Required bool
	Help     string
	Details  []string
}

const (
	retryInitialDuration = 500 * time.Millisecond
	retryFactor          = 2.0
	retryJitter          = 0.1
	retryMaxSteps        = 5
)

// RunPreUpgradeChecks runs pre-upgrade checks (permissions, cert-manager, codeflare-operator).
// When autoRemoveCodeflare is true, CodeFlare is automatically set to Removed if currently Managed.
func RunPreUpgradeChecks(ctx context.Context, c client.Client, autoRemoveCodeflare bool) []PreflightCheck {
	const numChecks = 3
	checks := make([]PreflightCheck, 0, numChecks)

	// Permission checks: list namespaces and list rayclusters as proxy for required access
	permOK, permDetails := checkPermissions(ctx, c)
	permMsg := "All required permissions granted"
	if !permOK {
		permMsg = "Missing permissions"
	}
	checks = append(checks, PreflightCheck{
		Name:     "Permissions",
		Passed:   permOK,
		Message:  permMsg,
		Required: true,
		Details:  permDetails,
	})

	// Cert-manager
	certOK, certMsg := checkCertManager(ctx, c)
	checks = append(checks, PreflightCheck{
		Name:     "cert-manager",
		Passed:   certOK,
		Message:  certMsg,
		Required: true,
		Help:     "cert-manager is required for RHOAI 3.x. Install it via OperatorHub before proceeding with the upgrade.",
	})

	// Codeflare-operator in DSC
	cfOK, cfMsg := checkCodeflareOperator(ctx, c, autoRemoveCodeflare)
	checks = append(checks, PreflightCheck{
		Name:     "codeflare-operator",
		Passed:   cfOK,
		Message:  cfMsg,
		Required: true,
		Help:     `Set codeflare to Removed in your DataScienceCluster before upgrading: oc patch datasciencecluster <name> --type merge -p '{"spec":{"components":{"codeflare":{"managementState":"Removed"}}}}'`,
	})

	return checks
}

func checkPermissions(ctx context.Context, c client.Client) (bool, []string) {
	var details []string
	_, errNS := c.List(ctx, resources.Namespace)
	if errNS != nil {
		details = append(details, "List namespaces: DENIED")
	} else {
		details = append(details, "List namespaces: OK")
	}
	_, errRC := c.List(ctx, resources.RayCluster)
	if errRC != nil {
		details = append(details, "List RayClusters: DENIED")
	} else {
		details = append(details, "List RayClusters: OK")
	}

	return errNS == nil && errRC == nil, details
}

func checkCertManager(ctx context.Context, c client.Client) (bool, string) {
	_, err := c.APIExtensions().ApiextensionsV1().CustomResourceDefinitions().Get(ctx, "certificates.cert-manager.io", metav1.GetOptions{})
	if err == nil {
		return true, "cert-manager CRD found"
	}
	if !apierrors.IsNotFound(err) {
		return false, "could not check cert-manager CRD: " + err.Error()
	}

	_, err = c.Get(ctx, resources.Namespace.GVR(), "cert-manager")
	if err == nil {
		return true, "cert-manager namespace found"
	}
	if !apierrors.IsNotFound(err) {
		return false, "could not check cert-manager namespace: " + err.Error()
	}

	_, err = c.Get(ctx, resources.Namespace.GVR(), "openshift-cert-manager")
	if err == nil {
		return true, "openshift-cert-manager namespace found"
	}
	if !apierrors.IsNotFound(err) {
		return false, "could not check openshift-cert-manager namespace: " + err.Error()
	}

	return false, "cert-manager not detected"
}

func checkCodeflareOperator(ctx context.Context, c client.Client, autoRemove bool) (bool, string) {
	dsc, err := client.GetDataScienceCluster(ctx, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, "DataScienceCluster CRD not found (OK to proceed)"
		}

		return false, "Could not check DataScienceCluster: " + err.Error()
	}

	dscName, _ := jq.Query[string](dsc, ".metadata.name")
	if dscName == "" {
		dscName = "unknown"
	}

	components, err := jq.Query[map[string]any](dsc, ".spec.components")
	if err != nil || components == nil {
		return true, "No components in DSC (OK to proceed)"
	}

	codeflare, _ := components["codeflare"].(map[string]any)
	if codeflare == nil {
		return true, "codeflare not present in DSC '" + dscName + "' (OK to proceed)"
	}

	state, _ := codeflare["managementState"].(string)
	state = strings.ToLower(state)

	switch state {
	case "removed":
		return true, "codeflare is Removed in DSC '" + dscName + "'"
	case "unmanaged":
		return true, "codeflare is Unmanaged in DSC '" + dscName + "'"
	case "managed":
		if autoRemove {
			if err := patchCodeflareToRemoved(ctx, c); err != nil {
				return false, fmt.Sprintf("failed to auto-remove codeflare in DSC '%s': %v", dscName, err)
			}

			return true, "Set codeflare to Removed in DataScienceCluster '" + dscName + "'"
		}

		return false, "codeflare is Managed in DSC '" + dscName + "' (should be Removed)"
	case "":
		return true, "codeflare present without managementState in DSC '" + dscName + "' (OK to proceed)"
	default:
		return false, "codeflare is '" + state + "' in DSC '" + dscName + "'"
	}
}

func patchCodeflareToRemoved(ctx context.Context, c client.Client) error {
	err := wait.ExponentialBackoffWithContext(ctx, wait.Backoff{
		Duration: retryInitialDuration,
		Factor:   retryFactor,
		Jitter:   retryJitter,
		Steps:    retryMaxSteps,
	}, func(ctx context.Context) (bool, error) {
		latestDSC, err := client.GetDataScienceCluster(ctx, c)
		if err != nil {
			return false, fmt.Errorf("getting DataScienceCluster: %w", err)
		}

		currentState, _ := jq.Query[string](latestDSC, ".spec.components.codeflare.managementState")
		currentState = strings.ToLower(currentState)

		if currentState == "removed" || currentState == "unmanaged" {
			return true, nil
		}

		if err := jq.Transform(latestDSC, `.spec.components.codeflare.managementState = "Removed"`); err != nil {
			return false, fmt.Errorf("setting codeflare managementState: %w", err)
		}

		gvk := latestDSC.GroupVersionKind()
		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: resources.DataScienceCluster.Resource,
		}

		_, err = c.Dynamic().Resource(gvr).
			Update(ctx, latestDSC, metav1.UpdateOptions{})
		if err != nil {
			if apierrors.IsConflict(err) {
				return false, nil
			}

			return false, fmt.Errorf("updating DataScienceCluster: %w", err)
		}

		return true, nil
	})
	if err != nil {
		return fmt.Errorf("patching codeflare to Removed: %w", err)
	}

	return nil
}
