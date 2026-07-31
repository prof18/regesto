// Package regesto carries the instance-side files that have to exist inside a
// knowledge base rather than inside the engine: the bin/ shims, the adapters
// (skills, hooks, instructions template) and the schema.
//
// They are embedded rather than copied from a source tree because the engine is
// distributed as a binary — a released `regesto` has no repository to copy from,
// and `regesto init` still has to produce an instance that works. Embedding is
// what lets the two live apart (PLAN 4.a).
//
// The templates keep their `{{kb_root}}` placeholder here; `bin/regesto-install`
// resolves it per instance.
package regesto

import (
	"embed"
	"io/fs"
	"strings"
)

// Adapters is the adapters/ tree: skills, the SessionStart hook and the
// instructions template. Dot-files are excluded, so the .gitkeep placeholders
// that keep empty vendor directories in git do not reach an instance.
//
//go:embed adapters
var Adapters embed.FS

// Shims are the bin/ entry points that hooks, skills and scheduled jobs call by
// a stable path. The built binary is deliberately not among them: it is
// per-platform, and an instance gets it from a release or a local build.
//
//go:embed bin/regesto-config bin/regesto-context bin/regesto-index bin/regesto-install bin/regesto-project bin/regesto-search
var Shims embed.FS

// Schema is the fact contract. It ships in the engine as the spec and lands in
// every instance as the document its write skill points agents at.
//
//go:embed SCHEMA.md
var Schema string

// Examples are the demo facts `regesto init --examples` copies in: an invented
// project with a controlled vocabulary worth imitating. An empty knowledge base
// gives an agent nothing to pattern-match against, and the first few facts set
// the vocabulary everything after them has to reuse.
//
//go:embed examples/facts
var Examples embed.FS

// InstanceFiles is every file the engine owns inside an instance, keyed by its
// path relative to the knowledge-base root.
//
// One list, used by both `init` (which writes what is absent) and `upgrade`
// (which refreshes what is stale). Two lists would drift, and the failure would
// be a file that init creates and upgrade never maintains.
//
// Deliberately excluded: config.toml and the ignore files. Those are scaffolded
// once and then belong to the instance — an engine that rewrote them would
// overwrite the user's own settings.
func InstanceFiles() (map[string][]byte, error) {
	out := map[string][]byte{"SCHEMA.md": []byte(Schema)}
	for _, src := range []fs.FS{Adapters, Shims} {
		err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			body, err := fs.ReadFile(src, p)
			if err != nil {
				return err
			}
			out[p] = body
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Executable reports whether an instance file needs the executable bit. Hooks
// and shims are run directly by agents and schedulers; nothing else is.
func Executable(path string) bool {
	return strings.HasSuffix(path, ".sh") || strings.HasPrefix(path, "bin/")
}
