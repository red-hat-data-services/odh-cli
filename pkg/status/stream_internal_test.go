package status

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	. "github.com/onsi/gomega"
)

func TestStreamJSON(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	report := &clusterhealth.Report{}

	err := streamJSON(&buf, report, nil)
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(HaveSuffix("\n"))
	g.Expect(strings.Count(output, "\n")).To(Equal(1))

	var parsed map[string]any
	g.Expect(json.Unmarshal(buf.Bytes(), &parsed)).To(Succeed())
	g.Expect(parsed).To(HaveKey("apiVersion"))
	g.Expect(parsed).To(HaveKey("report"))
}

func TestStreamJSON_MultipleCallsProduceNDJSON(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	report := &clusterhealth.Report{}

	g.Expect(streamJSON(&buf, report, nil)).To(Succeed())
	g.Expect(streamJSON(&buf, report, nil)).To(Succeed())

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	g.Expect(lines).To(HaveLen(2))

	for _, line := range lines {
		var parsed map[string]any
		g.Expect(json.Unmarshal([]byte(line), &parsed)).To(Succeed())
	}
}

func TestStreamYAML(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	report := &clusterhealth.Report{}

	err := streamYAML(&buf, report, nil)
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(HavePrefix("---\n"))
	g.Expect(output).To(ContainSubstring("apiVersion:"))
}

func TestStreamYAML_MultipleCallsProduceMultiDoc(t *testing.T) {
	g := NewWithT(t)

	var buf bytes.Buffer
	report := &clusterhealth.Report{}

	g.Expect(streamYAML(&buf, report, nil)).To(Succeed())
	g.Expect(streamYAML(&buf, report, nil)).To(Succeed())

	output := buf.String()
	g.Expect(strings.Count(output, "---\n")).To(Equal(2))
}
