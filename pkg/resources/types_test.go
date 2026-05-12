package resources_test

import (
	"testing"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

// TestGVKSyncWithClusterhealth ensures CLI ResourceType definitions stay in sync
// with clusterhealth library GVKs. If either side changes their GVK definitions,
// this test will fail and alert developers to the mismatch.
func TestGVKSyncWithClusterhealth(t *testing.T) {
	t.Run("DSCInitialization", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(resources.DSCInitialization.GVK()).To(Equal(clusterhealth.DSCInitializationGVK))
	})

	t.Run("DataScienceCluster", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(resources.DataScienceCluster.GVK()).To(Equal(clusterhealth.DataScienceClusterGVK))
	})
}
