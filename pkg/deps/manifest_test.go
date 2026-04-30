package deps_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/deps"

	. "github.com/onsi/gomega"
)

const testManifestWithDependencies = `
dependencies:
  certManager:
    enabled: "true"
    olm:
      name: cert-manager
      namespace: cert-manager
      channel: stable
  serviceMesh:
    enabled: "auto"
    olm:
      name: servicemeshoperator
      namespace: openshift-operators
components:
  kserve:
    dependencies:
      certManager: true
      serviceMesh: true
`

const testManifestEmpty = `
dependencies: {}
components: {}
`

const testManifestInvalid = `not: valid: yaml: here`

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
		check   func(*GomegaWithT, *deps.Manifest)
	}{
		{
			name:    "valid manifest with dependencies",
			data:    testManifestWithDependencies,
			wantErr: false,
			check: func(g *GomegaWithT, m *deps.Manifest) {
				g.Expect(m.Dependencies).To(HaveLen(2))
				g.Expect(m.Dependencies["certManager"].Enabled).To(Equal("true"))
				g.Expect(m.Dependencies["certManager"].OLM.Name).To(Equal("cert-manager"))
				g.Expect(m.Components).To(HaveLen(1))
			},
		},
		{
			name:    "empty manifest",
			data:    testManifestEmpty,
			wantErr: false,
			check: func(g *GomegaWithT, m *deps.Manifest) {
				g.Expect(m.Dependencies).To(BeEmpty())
			},
		},
		{
			name:    "invalid yaml",
			data:    testManifestInvalid,
			wantErr: true,
			check:   nil,
		},
		{
			name:    "empty data",
			data:    "",
			wantErr: false,
			check: func(g *GomegaWithT, m *deps.Manifest) {
				g.Expect(m.Dependencies).To(BeEmpty())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := deps.Parse([]byte(tt.data))

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())

				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			if tt.check != nil {
				tt.check(g, got)
			}
		})
	}
}

func TestGetDependencies(t *testing.T) {
	g := NewWithT(t)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"certManager": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "cert-manager",
					Namespace: "cert-manager",
					Channel:   "stable",
				},
			},
			"serviceMesh": {
				Enabled: "auto",
				OLM: deps.OLMConfig{
					Name:      "servicemeshoperator",
					Namespace: "openshift-operators",
				},
			},
		},
		Components: map[string]deps.Component{
			"kserve": {
				Dependencies: map[string]any{
					"certManager": true,
					"serviceMesh": true,
				},
			},
		},
	}

	depsInfo := manifest.GetDependencies()

	g.Expect(depsInfo).To(HaveLen(2))
	g.Expect(depsInfo[0].Name).To(Equal("certManager"))
	g.Expect(depsInfo[1].Name).To(Equal("serviceMesh"))
	g.Expect(depsInfo[0].DisplayName).To(Equal("Cert Manager"))
	g.Expect(depsInfo[0].RequiredBy).To(ContainElement("KServe"))
}

func TestGetDependencies_TransitiveDependencies(t *testing.T) {
	g := NewWithT(t)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"serviceMesh": {
				Enabled: "auto",
				OLM: deps.OLMConfig{
					Name:      "servicemeshoperator",
					Namespace: "openshift-operators",
				},
				Dependencies: map[string]any{
					"certManager": true,
				},
			},
			"certManager": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "cert-manager",
					Namespace: "cert-manager",
				},
			},
		},
		Components: map[string]deps.Component{},
	}

	depsInfo := manifest.GetDependencies()

	var certManager *deps.DependencyInfo
	for i := range depsInfo {
		if depsInfo[i].Name == "certManager" {
			certManager = &depsInfo[i]

			break
		}
	}

	g.Expect(certManager).ToNot(BeNil())
	g.Expect(certManager.RequiredBy).To(ContainElement("Service Mesh"))
}

func TestGetDependencies_NoDuplicatesInRequiredBy(t *testing.T) {
	g := NewWithT(t)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"certManager": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "cert-manager",
					Namespace: "cert-manager",
				},
			},
		},
		Components: map[string]deps.Component{
			"kserve": {
				Dependencies: map[string]any{
					"certManager": true,
				},
			},
			"modelregistry": {
				Dependencies: map[string]any{
					"certManager": true,
				},
			},
		},
	}

	depsInfo := manifest.GetDependencies()

	g.Expect(depsInfo).To(HaveLen(1))
	g.Expect(depsInfo[0].RequiredBy).To(HaveLen(2))
	g.Expect(depsInfo[0].RequiredBy[0]).To(Equal("KServe"))
	g.Expect(depsInfo[0].RequiredBy[1]).To(Equal("Model Registry"))
}

func TestGetDependencies_DisabledDependencies(t *testing.T) {
	g := NewWithT(t)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"certManager": {
				Enabled: "true",
				OLM:     deps.OLMConfig{Name: "cert-manager"},
			},
		},
		Components: map[string]deps.Component{
			"kserve": {
				Dependencies: map[string]any{
					"certManager": false,
				},
			},
		},
	}

	depsInfo := manifest.GetDependencies()

	g.Expect(depsInfo).To(HaveLen(1))
	g.Expect(depsInfo[0].RequiredBy).To(BeEmpty())
}

func TestGetDependencies_EmptyManifest(t *testing.T) {
	g := NewWithT(t)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{},
		Components:   map[string]deps.Component{},
	}

	depsInfo := manifest.GetDependencies()

	g.Expect(depsInfo).To(BeEmpty())
}

func TestNewDependencyManifestList_EnvelopeFields(t *testing.T) {
	g := NewWithT(t)

	depsInfo := []deps.DependencyInfo{
		{Name: "certManager", DisplayName: "Cert Manager", Enabled: "true"},
	}

	list := deps.NewDependencyManifestList(depsInfo)

	g.Expect(list.APIVersion).To(Equal("cli.opendatahub.io/v1"))
	g.Expect(list.Kind).To(Equal("DependencyManifestList"))
	g.Expect(list.Metadata.Command).To(Equal("deps"))
	g.Expect(list.Metadata.CLIVersion).ToNot(BeEmpty())
	g.Expect(list.Metadata.GeneratedAt).ToNot(BeEmpty())
	g.Expect(list.Status).To(BeNil())
	g.Expect(list.Dependencies).To(HaveLen(1))
}
