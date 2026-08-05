// Package toolconfig defines the trailer fields every per-tool config file
// carries (CLI version, semver config version, written-at timestamp) plus
// the Sample shape the refresh scanner reads from disk.
package toolconfig

import "time"

// Tool identifies one of the configurable build tools.
type Tool string

const (
	Gradle    Tool = "gradle"
	Bazel     Tool = "bazel"
	Xcelerate Tool = "xcelerate"
	Ccache    Tool = "ccache"
)

// Current config-schema versions per tool. Bump the MAJOR component when an
// activate-flow re-run is required; refresh.Notify only nudges across major
// bumps.
const (
	GradleConfigVersion    = "1.0.0"
	BazelConfigVersion     = "1.0.0"
	XcelerateConfigVersion = "1.0.0"
	CcacheConfigVersion    = "1.0.0"
)

// Sample is the subset of a tool's config that refresh cares about.
type Sample struct {
	Tool          Tool
	ConfigVersion string
	WrittenAt     time.Time
	ConfigPath    string
}
