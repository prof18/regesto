// Package version reports which engine is running.
//
// It reads Go's own build metadata rather than a string set by a release
// script: `go install ...@v0.1.0` stamps the module version, and a build from a
// checkout stamps the commit. Nothing has to be remembered at release time, and
// a version can never disagree with the binary it is compiled into.
package version

import "runtime/debug"

// stamped is set at link time for builds that have no VCS metadata to read:
//
//	go build -ldflags "-X github.com/prof18/regesto/internal/version.stamped=v0.1.0"
//
// Releases and package managers build from an extracted source archive with no
// .git, where Go can stamp nothing and the version would otherwise be "unknown".
var stamped string

// Current identifies this engine.
//
// Usually that is Go's own stamp: a release tag for `go install ...@v0.1.0`, or
// a pseudo-version carrying the commit and its date for a build from a checkout,
// suffixed "+dirty" when the tree had uncommitted changes. The commit fallback
// below only runs when VCS stamping was switched off, and "unknown" only when
// there is no build information at all.
func Current() string {
	if stamped != "" {
		return stamped
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		return "devel+" + rev + "-dirty"
	}
	return "devel+" + rev
}
