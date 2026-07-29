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

import "embed"

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
//go:embed bin/regesto-context bin/regesto-index bin/regesto-install bin/regesto-search
var Shims embed.FS

// Schema is the fact contract. It ships in the engine as the spec and lands in
// every instance as the document its write skill points agents at.
//
//go:embed SCHEMA.md
var Schema string
