// Package version holds build-time version metadata for the Tuck binary.
// The variables are populated via -ldflags at build time:
//
//	-X github.com/NAGenaev/tuck/internal/version.Version=1.31.0
//	-X github.com/NAGenaev/tuck/internal/version.Commit=abc1234
//	-X github.com/NAGenaev/tuck/internal/version.BuildDate=2026-06-13T00:00:00Z
package version

// Version is the semver release tag (e.g. "1.8.0").
// Defaults to "dev" when not set via ldflags.
var Version = "dev"

// Commit is the short git SHA of the build.
var Commit = "unknown"

// BuildDate is the RFC3339 timestamp of the build.
var BuildDate = "unknown"

// String returns a human-readable version string.
func String() string {
	if Commit == "unknown" {
		return Version
	}
	return Version + " (" + Commit + ")"
}
