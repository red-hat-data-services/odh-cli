package version

import "github.com/blang/semver/v4"

// IsUpgradeFrom2xTo3x checks if the versions represent an upgrade from 2.x to 3.x specifically.
// Future major versions (4.x+) may have different compatibility requirements.
// Returns false if either version is nil.
func IsUpgradeFrom2xTo3x(from *semver.Version, to *semver.Version) bool {
	if from == nil || to == nil {
		return false
	}

	return from.Major == 2 && to.Major == 3
}

// IsVersionAtLeast checks if the given version is at least the specified major.minor version.
// Patch version is ignored in the comparison.
// Returns false if version is nil.
func IsVersionAtLeast(
	version *semver.Version,
	major uint64,
	minor uint64,
) bool {
	if version == nil {
		return false
	}

	if version.Major > major {
		return true
	}

	return version.Major == major && version.Minor >= minor
}
