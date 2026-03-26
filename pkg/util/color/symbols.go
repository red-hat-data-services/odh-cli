package color

import "github.com/fatih/color"

// Pre-allocated Color objects. Created once at package level; Sprint/Sprintf
// is deferred to call time so that color.NoColor (set during Complete) is
// respected.
//
//nolint:gochecknoglobals
var (
	green      = color.New(color.FgGreen)
	yellow     = color.New(color.FgYellow)
	red        = color.New(color.FgRed)
	cyan       = color.New(color.FgCyan)
	redBold    = color.New(color.FgRed, color.Bold)
	greenBold  = color.New(color.FgGreen, color.Bold)
	yellowBold = color.New(color.FgYellow, color.Bold)
)

// StatusPass returns a green checkmark symbol.
func StatusPass() string {
	return green.Sprint("✓")
}

// StatusWarn returns a yellow warning symbol.
func StatusWarn() string {
	return yellow.Sprint("⚠")
}

// StatusFail returns a red cross symbol.
func StatusFail() string {
	return red.Sprint("✗")
}

// Severity level formatting.

// SeverityCritical returns "critical" in red.
func SeverityCritical() string {
	return red.Sprint("critical")
}

// SeverityWarning returns "warning" in yellow.
func SeverityWarning() string {
	return yellow.Sprint("warning")
}

// SeverityInfo returns "info" in cyan.
func SeverityInfo() string {
	return cyan.Sprint("info")
}

// VerdictFail returns "FAIL" in bold red.
func VerdictFail() string {
	return redBold.Sprint("FAIL")
}

// VerdictWarning returns "WARNING" in bold yellow.
func VerdictWarning() string {
	return yellowBold.Sprint("WARNING")
}

// VerdictPass returns "PASS" in bold green.
func VerdictPass() string {
	return greenBold.Sprint("PASS")
}

// StatusProhibited returns a bold red double-exclamation symbol.
func StatusProhibited() string {
	return redBold.Sprint("‼")
}

// SeverityProhibited returns "prohibited" in bold red.
func SeverityProhibited() string {
	return redBold.Sprint("prohibited")
}

// VerdictProhibited returns "PROHIBITED" in bold red.
func VerdictProhibited() string {
	return redBold.Sprint("PROHIBITED")
}

// BannerProhibited returns a bold red formatted string for prohibited banners.
func BannerProhibited(format string, a ...any) string {
	return redBold.Sprintf(format, a...)
}
