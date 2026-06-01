package rhbok_test

import (
	"bytes"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"

	. "github.com/onsi/gomega"
)

func TestRunTask_Validate(t *testing.T) {
	t.Run("runs all preflight checks", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cq := makeClusterQueue("test-cq")
		target := newTarget(t, []*unstructured.Unstructured{dsc, cq}, targetOpts{rbacAllowed: true})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Run()

		res, err := task.Validate(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).ToNot(BeNil())
		// checkCurrentKueueState + checkNoRHBOKConflicts + verifyKueueResources + verifyRBAC
		g.Expect(res.Status.Steps).To(HaveLen(4))
	})

	t.Run("reports RBAC failure", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{rbacAllowed: false})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Run()

		res, err := task.Validate(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		rbacStep := findStep(res.Status.Steps, "verify-rbac")
		g.Expect(rbacStep).ToNot(BeNil())
		g.Expect(rbacStep.Status).To(Equal(result.StepFailed))
	})
}

func TestRunTask_Execute(t *testing.T) {
	t.Run("dry-run reports all steps as skipped", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		cq := makeClusterQueue("test-cq")
		target := newTarget(t, []*unstructured.Unstructured{dsc, cm, cq}, targetOpts{
			dryRun:      true,
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Run()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).ToNot(BeNil())

		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).ToNot(BeNil())
		g.Expect(configStep.Status).To(Equal(result.StepSkipped))

		installStep := findStep(res.Status.Steps, "install-rhbok-operator")
		g.Expect(installStep).ToNot(BeNil())
		g.Expect(installStep.Status).To(Equal(result.StepSkipped))

		updateStep := findStep(res.Status.Steps, "update-datasciencecluster")
		g.Expect(updateStep).ToNot(BeNil())
		g.Expect(updateStep.Status).To(Equal(result.StepSkipped))

		verifyStep := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(verifyStep).ToNot(BeNil())
		g.Expect(verifyStep.Status).To(Equal(result.StepSkipped))
	})

	t.Run("skips preserveKueueConfig when Kueue is not managed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Unmanaged"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			dryRun:      true,
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Run()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).To(BeNil())
	})

	t.Run("DSC already Unmanaged skips update", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Unmanaged"))
		sub := makeSubscription(rhbok.ExportSubscriptionName, inNamespace(rhbok.ExportOperatorNamespace))
		target := newTarget(t, []*unstructured.Unstructured{dsc, sub}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			olmObjects: []runtime.Object{
				newOLMSubscription(rhbok.ExportSubscriptionName, rhbok.ExportOperatorNamespace),
				newOLMCSV(rhbok.ExportCSVNamePrefix+".v1.0.0", rhbok.ExportOperatorNamespace),
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Run()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		updateStep := findStep(res.Status.Steps, "update-datasciencecluster")
		g.Expect(updateStep).ToNot(BeNil())
		g.Expect(updateStep.Status).To(Equal(result.StepSkipped))
		g.Expect(updateStep.Message).To(ContainSubstring("already set to Unmanaged"))
	})

	t.Run("preserveKueueConfig annotates ConfigMap", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		target := newTarget(t, []*unstructured.Unstructured{dsc, cm}, targetOpts{
			dryRun:      true,
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportPreserveKueueConfig(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).ToNot(BeNil())
		g.Expect(configStep.Status).To(Equal(result.StepSkipped))
		g.Expect(configStep.Message).To(ContainSubstring("Dry-run"))
	})

	t.Run("preserveKueueConfig skips when ConfigMap not found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportPreserveKueueConfig(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).ToNot(BeNil())
		g.Expect(configStep.Status).To(Equal(result.StepSkipped))
		g.Expect(configStep.Message).To(ContainSubstring("not found"))
	})

	t.Run("verifyResourcesPreserved reports counts", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cq1 := makeClusterQueue("cq-1")
		cq2 := makeClusterQueue("cq-2")
		lq := makeLocalQueue("lq-1", inNamespace("default"))
		target := newTarget(t, []*unstructured.Unstructured{cq1, cq2, lq}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepCompleted))
		g.Expect(step.Message).To(ContainSubstring("2 ClusterQueues"))
		g.Expect(step.Message).To(ContainSubstring("1 LocalQueues"))
	})

	t.Run("verifyResourcesPreserved dry-run skips", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			dryRun:      true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepSkipped))
	})

	t.Run("updateDataScienceCluster updates to Unmanaged", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportUpdateDSC(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "update-datasciencecluster")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepCompleted))
		g.Expect(step.Message).To(ContainSubstring("updated successfully"))
	})

	t.Run("updateDataScienceCluster dry-run skips", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			dryRun:      true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportUpdateDSC(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "update-datasciencecluster")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepSkipped))
		g.Expect(step.Message).To(ContainSubstring("Would set"))
	})

	t.Run("updateDataScienceCluster fails when DSC not found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportUpdateDSC(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "update-datasciencecluster")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepFailed))
	})

	t.Run("preserveKueueConfig annotates ConfigMap in non-dry-run", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		target := newTarget(t, []*unstructured.Unstructured{cm}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportPreserveKueueConfig(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).ToNot(BeNil())
		g.Expect(configStep.Status).To(Equal(result.StepCompleted))
		g.Expect(configStep.Message).To(ContainSubstring("annotated for preservation"))
	})

	t.Run("installRHBOKOperator with existing subscription and CSV", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			olmObjects: []runtime.Object{
				newOLMSubscription(rhbok.ExportSubscriptionName, rhbok.ExportOperatorNamespace),
				newOLMCSV(rhbok.ExportCSVNamePrefix+".v1.0.0", rhbok.ExportOperatorNamespace),
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportInstallRHBOKOperator(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "install-rhbok-operator")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepCompleted))
		g.Expect(step.Message).To(ContainSubstring("already installed"))
	})

	t.Run("installRHBOKOperator dry-run no subscription", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			dryRun:      true,
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportInstallRHBOKOperator(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "install-rhbok-operator")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepSkipped))
	})

	t.Run("installRHBOKOperator dry-run with existing subscription and CSV", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			dryRun:      true,
			skipConfirm: true,
			rbacAllowed: true,
			olmObjects: []runtime.Object{
				newOLMSubscription(rhbok.ExportSubscriptionName, rhbok.ExportOperatorNamespace),
				newOLMCSV(rhbok.ExportCSVNamePrefix+".v1.0.0", rhbok.ExportOperatorNamespace),
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportInstallRHBOKOperator(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "install-rhbok-operator")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepSkipped))
	})

	t.Run("preserveKueueConfig annotates ConfigMap with existing annotations", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace), withAnnotation("existing-key", "existing-value"))
		target := newTarget(t, []*unstructured.Unstructured{cm}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportPreserveKueueConfig(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).ToNot(BeNil())
		g.Expect(configStep.Status).To(Equal(result.StepCompleted))

		annotateStep := findStepRecursive(res.Status.Steps, "apply-annotation")
		g.Expect(annotateStep).ToNot(BeNil())
		g.Expect(annotateStep.Status).To(Equal(result.StepCompleted))
	})

	t.Run("preserveKueueConfig with ConfigMap no annotations", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		target := newTarget(t, []*unstructured.Unstructured{cm}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportPreserveKueueConfig(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).ToNot(BeNil())
		g.Expect(configStep.Status).To(Equal(result.StepCompleted))
	})

	t.Run("verifyResourcesPreserved with ClusterQueues only", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cq := makeClusterQueue("test-cq")
		target := newTarget(t, []*unstructured.Unstructured{cq}, targetOpts{
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepCompleted))
		g.Expect(step.Message).To(ContainSubstring("1 ClusterQueues"))
		g.Expect(step.Message).To(ContainSubstring("0 LocalQueues"))
	})

	t.Run("verifyResourcesPreserved no resources returns completed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepCompleted))
	})

	t.Run("verifyResourcesPreserved ClusterQueue list error", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "clusterqueues" {
					return true, nil, errors.New("connection refused")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepFailed))
		g.Expect(step.Message).To(ContainSubstring("Failed to list ClusterQueues"))
	})

	t.Run("preserveKueueConfig update error fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		target := newTarget(t, []*unstructured.Unstructured{cm}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "configmaps" && act.GetVerb() == "update" {
					return true, nil, errors.New("update forbidden")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportPreserveKueueConfig(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).ToNot(BeNil())
		g.Expect(configStep.Status).To(Equal(result.StepFailed))
	})

	t.Run("updateDataScienceCluster update error fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "datascienceclusters" && act.GetVerb() == "update" {
					return true, nil, errors.New("update forbidden")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportUpdateDSC(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "update-datasciencecluster")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepFailed))
	})

	t.Run("installRHBOKOperator user cancels prompt", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		inBuf := bytes.NewBufferString("n\n")
		target := newTarget(t, nil, targetOpts{
			rbacAllowed: true,
		})
		target.IO = iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportInstallRHBOKOperator(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "install-rhbok-operator")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepSkipped))
		g.Expect(step.Message).To(ContainSubstring("cancelled"))
	})

	t.Run("updateDataScienceCluster user cancels prompt", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		inBuf := bytes.NewBufferString("n\n")
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			rbacAllowed: true,
		})
		target.IO = iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportUpdateDSC(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "update-datasciencecluster")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepSkipped))
		g.Expect(step.Message).To(ContainSubstring("cancelled"))
	})

	t.Run("full flow with existing operator", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		cq := makeClusterQueue("cq-1")
		lq := makeLocalQueue("lq-1", inNamespace("default"))
		sub := makeSubscription(rhbok.ExportSubscriptionName, inNamespace(rhbok.ExportOperatorNamespace))
		target := newTarget(t, []*unstructured.Unstructured{dsc, cm, cq, lq, sub}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			olmObjects: []runtime.Object{
				newOLMSubscription(rhbok.ExportSubscriptionName, rhbok.ExportOperatorNamespace),
				newOLMCSV(rhbok.ExportCSVNamePrefix+".v1.0.0", rhbok.ExportOperatorNamespace),
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Run()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).ToNot(BeNil())
		g.Expect(configStep.Status).To(Equal(result.StepCompleted))

		installStep := findStep(res.Status.Steps, "install-rhbok-operator")
		g.Expect(installStep).ToNot(BeNil())
		g.Expect(installStep.Status).To(Equal(result.StepCompleted))

		updateStep := findStep(res.Status.Steps, "update-datasciencecluster")
		g.Expect(updateStep).ToNot(BeNil())
		g.Expect(updateStep.Status).To(Equal(result.StepCompleted))

		verifyStep := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(verifyStep).ToNot(BeNil())
		g.Expect(verifyStep.Status).To(Equal(result.StepCompleted))
	})

	t.Run("verifyResourcesPreserved LocalQueue list error", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cq := makeClusterQueue("cq-1")
		target := newTarget(t, []*unstructured.Unstructured{cq}, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "localqueues" {
					return true, nil, errors.New("connection refused")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepFailed))
		g.Expect(step.Message).To(ContainSubstring("Failed to list LocalQueues"))
	})

	t.Run("verifyResourcesPreserved ClusterQueue NotFound returns completed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "clusterqueues" {
					return true, nil, apierrors.NewNotFound(
						schema.GroupResource{Group: "kueue.x-k8s.io", Resource: "clusterqueues"}, "")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepCompleted))
		g.Expect(step.Message).To(ContainSubstring("No ClusterQueue CRD found"))
	})

	t.Run("verifyResourcesPreserved LocalQueue NotFound returns completed with CQ count", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cq := makeClusterQueue("cq-1")
		target := newTarget(t, []*unstructured.Unstructured{cq}, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "localqueues" {
					return true, nil, apierrors.NewNotFound(
						schema.GroupResource{Group: "kueue.x-k8s.io", Resource: "localqueues"}, "")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepCompleted))
		g.Expect(step.Message).To(ContainSubstring("No LocalQueue CRD found"))
		g.Expect(step.Message).To(ContainSubstring("1 ClusterQueues"))
	})

	t.Run("updateDataScienceCluster verifies DSC state mutated", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportUpdateDSC(a, ctx, target)

		fetched, err := target.Client.Dynamic().Resource(resources.DataScienceClusterV1.GVR()).
			Get(ctx, "default-dsc", metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		state, err := jq.Query[string](fetched, ".spec.components.kueue.managementState")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(state).To(Equal("Unmanaged"))
	})

	t.Run("preserveKueueConfig verifies annotation written", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		target := newTarget(t, []*unstructured.Unstructured{cm}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportPreserveKueueConfig(a, ctx, target)

		fetched, err := target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
			Namespace(rhbok.ExportApplicationsNamespace).
			Get(ctx, rhbok.ExportConfigMapName, metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		annotations := fetched.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("opendatahub.io/managed", "false"))
	})

	t.Run("preserveKueueConfig preserves existing annotations", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace), withAnnotation("existing-key", "existing-value"))
		target := newTarget(t, []*unstructured.Unstructured{cm}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportPreserveKueueConfig(a, ctx, target)

		fetched, err := target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
			Namespace(rhbok.ExportApplicationsNamespace).
			Get(ctx, rhbok.ExportConfigMapName, metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		annotations := fetched.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("opendatahub.io/managed", "false"))
		g.Expect(annotations).To(HaveKeyWithValue("existing-key", "existing-value"))
	})

	t.Run("dry-run does not mutate DSC state", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		cq := makeClusterQueue("test-cq")
		target := newTarget(t, []*unstructured.Unstructured{dsc, cm, cq}, targetOpts{
			dryRun:      true,
			skipConfirm: true,
			rbacAllowed: true,
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Run()

		_, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		fetchedDSC, err := target.Client.Dynamic().Resource(resources.DataScienceClusterV1.GVR()).
			Get(ctx, "default-dsc", metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		state, err := jq.Query[string](fetchedDSC, ".spec.components.kueue.managementState")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(state).To(Equal("Managed"))

		fetchedCM, err := target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
			Namespace(rhbok.ExportApplicationsNamespace).
			Get(ctx, rhbok.ExportConfigMapName, metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(fetchedCM.GetAnnotations()).ToNot(HaveKey("opendatahub.io/managed"))
	})

	t.Run("full flow verifies k8s state after execution", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		cq := makeClusterQueue("cq-1")
		lq := makeLocalQueue("lq-1", inNamespace("default"))
		sub := makeSubscription(rhbok.ExportSubscriptionName, inNamespace(rhbok.ExportOperatorNamespace))
		target := newTarget(t, []*unstructured.Unstructured{dsc, cm, cq, lq, sub}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			olmObjects: []runtime.Object{
				newOLMSubscription(rhbok.ExportSubscriptionName, rhbok.ExportOperatorNamespace),
				newOLMCSV(rhbok.ExportCSVNamePrefix+".v1.0.0", rhbok.ExportOperatorNamespace),
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Run()

		_, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		// DSC should now have Kueue set to Unmanaged
		fetchedDSC, err := target.Client.Dynamic().Resource(resources.DataScienceClusterV1.GVR()).
			Get(ctx, "default-dsc", metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		state, err := jq.Query[string](fetchedDSC, ".spec.components.kueue.managementState")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(state).To(Equal("Unmanaged"))

		// ConfigMap should have the preservation annotation
		fetchedCM, err := target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
			Namespace(rhbok.ExportApplicationsNamespace).
			Get(ctx, rhbok.ExportConfigMapName, metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(fetchedCM.GetAnnotations()).To(HaveKeyWithValue("opendatahub.io/managed", "false"))

		// Kueue resources should still exist
		cqs, err := target.Client.ListResources(ctx, resources.ClusterQueue.GVR())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cqs).To(HaveLen(1))

		lqs, err := target.Client.ListResources(ctx, resources.LocalQueue.GVR())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(lqs).To(HaveLen(1))
	})

	t.Run("idempotent: second run on already-migrated cluster is a no-op", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		cq := makeClusterQueue("cq-1")
		sub := makeSubscription(rhbok.ExportSubscriptionName, inNamespace(rhbok.ExportOperatorNamespace))
		target := newTarget(t, []*unstructured.Unstructured{dsc, cm, cq, sub}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			olmObjects: []runtime.Object{
				newOLMSubscription(rhbok.ExportSubscriptionName, rhbok.ExportOperatorNamespace),
				newOLMCSV(rhbok.ExportCSVNamePrefix+".v1.0.0", rhbok.ExportOperatorNamespace),
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Run()

		// First run — performs the migration
		res1, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		updateStep1 := findStep(res1.Status.Steps, "update-datasciencecluster")
		g.Expect(updateStep1).ToNot(BeNil())
		g.Expect(updateStep1.Status).To(Equal(result.StepCompleted))

		// Second run on the same target — DSC is already Unmanaged, operator already installed
		target2 := newTarget(t, nil, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			olmObjects: []runtime.Object{
				newOLMSubscription(rhbok.ExportSubscriptionName, rhbok.ExportOperatorNamespace),
				newOLMCSV(rhbok.ExportCSVNamePrefix+".v1.0.0", rhbok.ExportOperatorNamespace),
			},
		})
		// Re-use the same dynamic client so we see the mutated state from run 1
		target2.Client = target.Client
		target2.IO = target.IO

		task2 := a.Run()
		res2, err := task2.Execute(ctx, target2)
		g.Expect(err).ToNot(HaveOccurred())

		// DSC update should be skipped — already Unmanaged
		updateStep2 := findStep(res2.Status.Steps, "update-datasciencecluster")
		g.Expect(updateStep2).ToNot(BeNil())
		g.Expect(updateStep2.Status).To(Equal(result.StepSkipped))
		g.Expect(updateStep2.Message).To(ContainSubstring("already set to Unmanaged"))

		// Operator install should report already installed
		installStep2 := findStep(res2.Status.Steps, "install-rhbok-operator")
		g.Expect(installStep2).ToNot(BeNil())
		g.Expect(installStep2.Status).To(Equal(result.StepCompleted))
		g.Expect(installStep2.Message).To(ContainSubstring("already installed"))

		// Resources should still be preserved
		verifyStep2 := findStep(res2.Status.Steps, "verify-resources-preserved")
		g.Expect(verifyStep2).ToNot(BeNil())
		g.Expect(verifyStep2.Status).To(Equal(result.StepCompleted))

		// Verify final k8s state is unchanged from first run
		fetchedDSC, err := target.Client.Dynamic().Resource(resources.DataScienceClusterV1.GVR()).
			Get(ctx, "default-dsc", metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		state, err := jq.Query[string](fetchedDSC, ".spec.components.kueue.managementState")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(state).To(Equal("Unmanaged"))
	})

	t.Run("preserveKueueConfig get error", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "configmaps" && act.GetVerb() == "get" {
					return true, nil, errors.New("server error")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		rhbok.ExportPreserveKueueConfig(a, ctx, target)

		res := target.Recorder.(interface{ Build() *result.ActionResult }).Build()
		configStep := findStep(res.Status.Steps, "preserve-kueue-config")
		g.Expect(configStep).ToNot(BeNil())
		g.Expect(configStep.Status).To(Equal(result.StepSkipped))
	})
}
