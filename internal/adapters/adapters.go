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
	"sort"
	"strconv"
	"strings"

	"github.com/prof18/regesto/internal/config"
)

// Agent is one agent's install targets. Paths are declared locations; they may
// themselves be symlinks, which callers should resolve before comparing.
type Agent struct {
	Name string `json:"name"`
	// ProfileID and DisplayName identify the declarative profile that produced
	// this integration. They are empty/name for a legacy unknown agent.
	ProfileID          string         `json:"profile_id"`
	DisplayName        string         `json:"display_name"`
	Detect             Detection      `json:"detect"`
	SkillsDirs         []string       `json:"skills_dirs"`
	SkillsVariant      string         `json:"skills_variant"`
	InstructionsFiles  []string       `json:"instructions_files"`
	InstructionsCreate bool           `json:"instructions_create"`
	Hooks              []Hook         `json:"hooks"`
	MemorySources      []MemorySource `json:"memory_sources"`
	DefaultTrust       string         `json:"default_trust"`
	// SkillsDir is where SKILL.md directories are linked.
	SkillsDir string `json:"skills_dir"`
	// InstructionsFile is the always-loaded instructions file that gets the
	// KB section (PLAN 1.e).
	InstructionsFile string `json:"instructions_file"`
	// SettingsFile is where hooks are registered, empty for agents that
	// have no hook mechanism.
	SettingsFile string `json:"settings_file"`
	// MemoryGlob is the legacy compatibility projection of the first declared
	// markdown-glob-v1 source. New harvesting consumes MemorySources directly.
	MemoryGlob string `json:"memory_glob"`
	// MaxCaptureBytes skips native files larger than this. It is a guard
	// against a pathological file, not a content filter — skipping a capture
	// is a silent way of losing knowledge, which this project's central rule
	// forbids. Override with [harvest].max_capture_bytes.
	MaxCaptureBytes int64 `json:"max_capture_bytes"`
	// ExcludeGlobs are filename patterns never captured. This is the right
	// control for "not content": size is only a proxy for it, and a bad one —
	// a large file can be the most valuable thing in the store. Override with
	// [harvest_exclude].<agent>, comma-separated.
	ExcludeGlobs []string `json:"exclude_globs"`
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

// For returns one Agent per name in the config's agent list, applying
// [skills_dirs] and [instructions] overrides over the vendor defaults. An agent
// with no default and no override still comes back, with empty paths, so the
// caller can report it as unknown rather than silently skipping it.
func For(cfg *config.Config) []Agent {
	out, err := Resolve(cfg)
	if err != nil {
		// Compatibility callers predate validation errors. CLI startup calls
		// Resolve explicitly; this wrapper preserves its old no-error signature.
		return nil
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

// KnownAgents returns the profile IDs this engine can detect automatically,
// sorted. It is the set `regesto init` checks for on the machine, and the set
// `regesto upgrade` compares a live instance's integrations against to notice one
// it did not exist to detect when the instance was created.
func KnownAgents() []string {
	profiles, err := embeddedProfiles()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(profiles))
	for name, p := range profiles {
		if len(p.Detect.Paths) == 0 {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Detect reports which known integrations are present on this machine using
// their declarative path signals. Run here so `regesto init` can propose an
// `integrations` list that already matches the machine instead of a fixed pair,
// and so `regesto upgrade` can notice an agent that showed up after the instance
// was created.
func Detect() []string {
	profiles, err := embeddedProfiles()
	if err != nil {
		return nil
	}
	var out []string
	for _, name := range KnownAgents() {
		p := profiles[name]
		for _, path := range p.Detect.Paths {
			if _, err := os.Stat(expandHome(path)); err == nil {
				out = append(out, name)
				break
			}
		}
	}
	return out
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
