package migrate

import (
	"bytes"
	"testing"

	"github.com/blang/semver/v4"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"

	. "github.com/onsi/gomega"
)

func newTestListCommand() (*ListCommand, *bytes.Buffer) {
	outBuf := &bytes.Buffer{}
	streams := genericiooptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    outBuf,
		ErrOut: &bytes.Buffer{},
	}

	cmd := &ListCommand{
		SharedOptions: NewSharedOptions(streams),
		registry:      action.NewActionRegistry(),
	}

	return cmd, outBuf
}

func TestBuildRows(t *testing.T) {
	current := semver.MustParse("2.25.0")
	target := semver.MustParse("3.0.0")

	t.Run("should include applicable actions", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestListCommand()
		cmd.parsedTargetVersion = &target

		actions := []action.Action{
			&stubAction{id: "test.migrate", phase: action.PhasePreUpgrade, canApply: true},
		}

		rows := cmd.buildRows(actions, &current)
		g.Expect(rows).To(HaveLen(1))
		g.Expect(rows[0].ID).To(Equal("test.migrate"))
		g.Expect(rows[0].Applicable).To(Equal("Yes"))
		g.Expect(rows[0].Phase).To(Equal("pre-upgrade"))
	})

	t.Run("should exclude non-applicable actions when ShowAll is false", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestListCommand()
		cmd.parsedTargetVersion = &target

		actions := []action.Action{
			&stubAction{id: "applicable", phase: action.PhasePreUpgrade, canApply: true},
			&stubAction{id: "not-applicable", phase: action.PhasePreUpgrade, canApply: false},
		}

		rows := cmd.buildRows(actions, &current)
		g.Expect(rows).To(HaveLen(1))
		g.Expect(rows[0].ID).To(Equal("applicable"))
	})

	t.Run("should include non-applicable actions when ShowAll is true", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestListCommand()
		cmd.ShowAll = true
		cmd.parsedTargetVersion = &target

		actions := []action.Action{
			&stubAction{id: "applicable", phase: action.PhasePreUpgrade, canApply: true},
			&stubAction{id: "not-applicable", phase: action.PhasePreUpgrade, canApply: false},
		}

		rows := cmd.buildRows(actions, &current)
		g.Expect(rows).To(HaveLen(2))
		g.Expect(rows[0].Applicable).To(Equal("Yes"))
		g.Expect(rows[1].Applicable).To(Equal("No"))
	})

	t.Run("should show N/A when ShowAll with no target version", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestListCommand()
		cmd.ShowAll = true
		cmd.parsedTargetVersion = nil

		actions := []action.Action{
			&stubAction{id: "test.migrate", phase: action.PhasePostUpgrade, canApply: false},
		}

		rows := cmd.buildRows(actions, nil)
		g.Expect(rows).To(HaveLen(1))
		g.Expect(rows[0].Applicable).To(Equal("N/A"))
	})
}

func TestPrintPhaseHint(t *testing.T) {
	current := semver.MustParse("2.25.0")
	target := semver.MustParse("3.0.0")

	t.Run("should show phase hint when phase is empty and format is table", func(t *testing.T) {
		g := NewWithT(t)
		cmd, outBuf := newTestListCommand()
		cmd.Phase = ""
		cmd.OutputFormat = OutputFormatTable
		cmd.parsedTargetVersion = &target

		cmd.printPhaseHint(&current)

		g.Expect(outBuf.String()).To(ContainSubstring("auto-detect phase as \"pre-upgrade\""))
	})

	t.Run("should not show phase hint when phase is explicitly set", func(t *testing.T) {
		g := NewWithT(t)
		cmd, outBuf := newTestListCommand()
		cmd.Phase = "pre-upgrade"
		cmd.OutputFormat = OutputFormatTable
		cmd.parsedTargetVersion = &target

		cmd.printPhaseHint(&current)

		g.Expect(outBuf.String()).To(BeEmpty())
	})

	t.Run("should not show phase hint for JSON output", func(t *testing.T) {
		g := NewWithT(t)
		cmd, outBuf := newTestListCommand()
		cmd.Phase = ""
		cmd.OutputFormat = OutputFormatJSON
		cmd.parsedTargetVersion = &target

		cmd.printPhaseHint(&current)

		g.Expect(outBuf.String()).To(BeEmpty())
	})

	t.Run("should not show phase hint when currentVersion is nil", func(t *testing.T) {
		g := NewWithT(t)
		cmd, outBuf := newTestListCommand()
		cmd.Phase = ""
		cmd.OutputFormat = OutputFormatTable
		cmd.parsedTargetVersion = &target

		cmd.printPhaseHint(nil)

		g.Expect(outBuf.String()).To(BeEmpty())
	})
}
