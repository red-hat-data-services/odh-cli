package status

// Export internal functions for testing.
//
//nolint:gochecknoglobals // Test exports only compiled in test builds
var (
	IsRBACError  = isRBACError
	StatusSymbol = statusSymbol
	VisibleLen   = visibleLen
	PadRight     = padRight
	Truncate     = truncate
)
