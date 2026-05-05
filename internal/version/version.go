package version

// Version information set by ldflags at build time.
//
//nolint:gochecknoglobals // These variables are set by ldflags during build.
var (
	// Version is the semantic version of the CLI.
	Version = "dev"
	// Commit is the git commit hash.
	Commit = "unknown"
	// Date is the build date.
	Date = "unknown"
)

// GetVersion returns the current version string.
func GetVersion() string {
	return Version
}

// GetCommit returns the git commit hash.
func GetCommit() string {
	return Commit
}

// GetDate returns the build date.
func GetDate() string {
	return Date
}

// Info holds version information for JSON/YAML output.
type Info struct {
	Version string `json:"version" jsonschema:"description=CLI version string"`
	Commit  string `json:"commit"  jsonschema:"description=Git commit hash"`
	Date    string `json:"date"    jsonschema:"description=Build date"`
}

// VerboseInfo holds extended version information including Go runtime details.
type VerboseInfo struct {
	Version   string `json:"version"   jsonschema:"description=CLI version string"`
	Commit    string `json:"commit"    jsonschema:"description=Git commit hash"`
	Date      string `json:"date"      jsonschema:"description=Build date"`
	GoVersion string `json:"goVersion" jsonschema:"description=Go runtime version"`
	Platform  string `json:"platform"  jsonschema:"description=OS and architecture"`
}

// GetInfo returns the version info struct.
func GetInfo() Info {
	return Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}
