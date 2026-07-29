// Package adapters resolves where each agent keeps its skills, instructions and
// settings on this machine.
//
// These are per-vendor conventions, not personal facts, so they ship as
// defaults — a fresh instance works before anything is configured. But every one
// is overridable from config.toml, because real setups diverge: an agent
// directory may be a symlink into a shared home, or instructions may live in a
// dotfiles repo. Nothing here may hardcode a path belonging to one person
// (PLAN §0 "two audiences", 4.b "config over convention").
package adapters

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"regesto/internal/config"
)

// Agent is one agent's install targets. Paths are declared locations; they may
// themselves be symlinks, which callers should resolve before comparing.
type Agent struct {
	Name string
	// SkillsDir is where SKILL.md directories are linked.
	SkillsDir string
	// InstructionsFile is the always-loaded instructions file that gets the
	// KB section (PLAN 1.e).
	InstructionsFile string
	// SettingsFile is where hooks are registered, empty for agents that
	// have no hook mechanism.
	SettingsFile string
	// MemoryGlob matches the agent's native memory directories, which
	// harvest reads and diffs (PLAN 2.a). Claude Code keeps one per project,
	// so this is a glob rather than a path.
	MemoryGlob string
	// MaxCaptureBytes skips native files larger than this. It is a guard
	// against a pathological file, not a content filter — skipping a capture
	// is a silent way of losing knowledge, which this project's central rule
	// forbids. Override with [harvest].max_capture_bytes.
	MaxCaptureBytes int64
	// ExcludeGlobs are filename patterns never captured. This is the right
	// control for "not content": size is only a proxy for it, and a bad one —
	// a large file can be the most valuable thing in the store. Override with
	// [harvest_exclude].<agent>, comma-separated.
	ExcludeGlobs []string
}

type defaults struct {
	skills, instructions, settings, memory string
	exclude                                []string
}

// defaultMaxCaptureBytes is deliberately generous. An earlier 64KB limit was set
// to stop Codex's ~1.3MB of regenerated summaries flooding the inbox, but the
// evidence did not support it: those files had not changed in 12 days while
// Codex was actively writing. They are bulky and *static*, not churning. Since
// harvest only captures on change, the cost of keeping them is a rare 1.3MB
// capture — against silently dropping the durable preferences that live only in
// MEMORY.md. The real cost of a large capture is normalisation reading it, which
// belongs in 2.b as a diff against the previously archived copy, not here as an
// all-or-nothing byte test.
const defaultMaxCaptureBytes = 10 * 1024 * 1024

// Per-vendor defaults. Adding a fourth agent should touch only this table
// (DECISION §7) plus its config override, if any.
var vendorDefaults = map[string]defaults{
	"claude": {
		skills:       "~/.claude/skills",
		instructions: "~/.claude/CLAUDE.md",
		settings:     "~/.claude/settings.json",
		// One memory directory per project, keyed by encoded absolute path.
		memory: "~/.claude/projects/*/memory",
	},
	"codex": {
		skills:       "~/.codex/skills",
		instructions: "~/.codex/AGENTS.md",
		memory:       "~/.codex/memories",
		// Raw per-session transcript dump, ~862KB and regenerated wholesale.
		// Codex already versions it in its own .git, and it holds no distilled
		// claim that MEMORY.md and memory_summary.md do not already carry.
		exclude: []string{"raw_memories.md"},
	},
	"hermes": {
		skills:       "~/.hermes/skills",
		instructions: "~/.hermes/SOUL.md",
		memory:       "~/.hermes/memories",
	},
}

// For returns one Agent per name in the config's agent list, applying
// [skills_dirs] and [instructions] overrides over the vendor defaults. An agent
// with no default and no override still comes back, with empty paths, so the
// caller can report it as unknown rather than silently skipping it.
func For(cfg *config.Config) []Agent {
	skillOverrides := cfg.Section("skills_dirs")
	instrOverrides := cfg.Section("instructions")
	settingOverrides := cfg.Section("settings_files")
	memoryOverrides := cfg.Section("memory_dirs")

	out := make([]Agent, 0, len(cfg.Agents))
	for _, name := range cfg.Agents {
		d := vendorDefaults[name]
		a := Agent{
			Name:             name,
			SkillsDir:        pick(skillOverrides[name], d.skills),
			InstructionsFile: pick(instrOverrides[name], d.instructions),
			SettingsFile:     pick(settingOverrides[name], d.settings),
			MemoryGlob:       pick(memoryOverrides[name], d.memory),
			MaxCaptureBytes:  maxCaptureBytes(cfg),
			ExcludeGlobs:     excludes(cfg, name, d.exclude),
		}
		out = append(out, a)
	}
	return out
}

// maxCaptureBytes reads [harvest].max_capture_bytes, falling back to the
// default. A value of 0 disables the guard entirely.
func maxCaptureBytes(cfg *config.Config) int64 {
	raw := strings.TrimSpace(cfg.Section("harvest")["max_capture_bytes"])
	if raw == "" {
		return defaultMaxCaptureBytes
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return defaultMaxCaptureBytes
	}
	return n
}

// excludes returns the never-capture patterns for an agent: the configured list
// if present, otherwise the vendor default.
func excludes(cfg *config.Config, agent string, def []string) []string {
	raw := strings.TrimSpace(cfg.Section("harvest_exclude")[agent])
	if raw == "" {
		return def
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// pick prefers a configured value over the vendor default, expanding ~.
func pick(override, def string) string {
	if override != "" {
		return expandHome(override)
	}
	return expandHome(def)
}

func expandHome(p string) string {
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return p
}
