// Package config loads the instance configuration (config.toml at the KB
// root). Per the "build for two audiences" rule (PLAN §0), every value that
// is machine- or person-specific — KB root, machine name, agent list,
// project map — comes from this file, never from constants in code.
//
// The parser handles the flat TOML subset the config actually uses: comments,
// `key = "string"`, `key = ["a", "b"]`, and `[table]` sections of string
// pairs. Zero dependencies keeps the binary a single static artifact with no
// module fetches (decision 0.11).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FileName = "config.toml"

type Config struct {
	// KBRoot is the knowledge-base root. Defaults to the directory
	// containing the config file, so a relocated instance keeps working.
	KBRoot string
	// Machine is this machine's short name (PLAN 0.1), e.g. "studio". Resolved
	// per-machine — see resolveMachine — never simply read from the synced
	// config file.
	Machine string
	// MachineSource records where Machine came from, so an install can show
	// its working instead of silently guessing.
	MachineSource string
	// Agents lists the agents whose adapters/harvest this instance runs.
	Agents []string
	// Projects maps repo basenames or absolute paths to canonical project
	// names (PLAN 0.2), covering remote-less repos and basename collisions.
	// Shorthand for Section("projects").
	Projects map[string]string
	// Sections holds every `[table]` of string pairs in the file, so a new
	// section is a config change rather than a parser change. Known
	// sections: projects, skills_dirs, instructions.
	Sections map[string]map[string]string
	// Path is where the config file was loaded from.
	Path string
}

// Section returns a `[table]` of string pairs, or an empty map if absent.
// Callers treat missing config as "use the default", never as an error — a
// stranger's instance should work before they have configured anything.
func (c *Config) Section(name string) map[string]string {
	if s, ok := c.Sections[name]; ok {
		return s
	}
	return map[string]string{}
}

// Find locates the instance config, in order: REGESTO_CONFIG, then walking up
// from startDir, then walking up from the binary's own location.
//
// The last fallback is what makes `regesto` work from any directory. The binary
// lives at <kb-root>/bin/regesto, so its own path identifies its instance —
// without this, running it by absolute path or through a symlink on PATH fails
// from anywhere outside the KB. Walking up from the working directory still
// comes first, so `cd`-ing into one instance uses that one even when another is
// on PATH.
func Find(startDir string) (string, error) {
	if p := os.Getenv("REGESTO_CONFIG"); p != "" {
		return p, nil
	}
	if found, err := walkUp(startDir); err == nil {
		return found, nil
	}
	if exe, err := os.Executable(); err == nil {
		// Resolve first: on macOS this may be the symlink used to invoke it,
		// and the instance is beside the real file, not the link.
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		if found, err := walkUp(filepath.Dir(exe)); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("no %s found, neither walking up from %s nor beside the binary "+
		"(set REGESTO_CONFIG or pass --config)", FileName, startDir)
}

func walkUp(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, FileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s above %s", FileName, startDir)
		}
		dir = parent
	}
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Projects: map[string]string{},
		Sections: map[string]map[string]string{},
		Path:     abs,
	}
	cfg.Sections["projects"] = cfg.Projects

	section := ""
	for lineNo, line := range strings.Split(string(raw), "\n") {
		line = stripComment(line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: not a key = value line: %q", path, lineNo+1, line)
		}
		key = unquote(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		switch section {
		case "":
			switch key {
			case "kb_root":
				cfg.KBRoot = expandHome(unquote(value))
			case "machine":
				cfg.Machine = unquote(value)
			case "agents":
				list, err := parseList(value)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: %w", path, lineNo+1, err)
				}
				cfg.Agents = list
			default:
				return nil, fmt.Errorf("%s:%d: unknown key %q", path, lineNo+1, key)
			}
		default:
			// Any `[table]` of string pairs is accepted, so adding a
			// section to an instance's config never requires a parser
			// change (PLAN 4.b, "config over convention"). Paths are
			// home-expanded here so every consumer sees absolute values.
			if _, ok := cfg.Sections[section]; !ok {
				cfg.Sections[section] = map[string]string{}
			}
			v := unquote(value)
			if strings.HasPrefix(v, "~") {
				v = expandHome(v)
			}
			cfg.Sections[section][key] = v
		}
	}

	if cfg.KBRoot == "" {
		cfg.KBRoot = filepath.Dir(abs)
	}
	cfg.Machine, cfg.MachineSource = resolveMachine(cfg)
	return cfg, nil
}

// MachineFile is the per-machine identity file, inside the unsynced .state/
// directory.
const MachineFile = "machine"

// resolveMachine determines this machine's short name.
//
// config.toml lives at the KB root, which is *synced* — so a static
// `machine = "..."` there is the same value on every machine, and cannot be the
// source of truth for a multi-machine instance. Identity therefore lives in
// `.state/`, which is excluded from both git and Syncthing precisely because it
// is per-machine (PLAN 0.a).
//
// Order: REGESTO_MACHINE, then .state/machine, then config.toml's `machine`
// (correct only for a single-machine instance), then a name derived from the
// hostname as a last resort.
func resolveMachine(cfg *Config) (string, string) {
	return ResolveMachineFor(cfg)
}

// ResolveMachineFor is resolveMachine, exported so `regesto init` derives the
// same name the rest of the tool will.
func ResolveMachineFor(cfg *Config) (string, string) {
	if v := strings.TrimSpace(os.Getenv("REGESTO_MACHINE")); v != "" {
		return v, "env:REGESTO_MACHINE"
	}
	statePath := filepath.Join(cfg.KBRoot, ".state", MachineFile)
	if raw, err := os.ReadFile(statePath); err == nil {
		if v := strings.TrimSpace(string(raw)); v != "" {
			return v, ".state/" + MachineFile
		}
	}
	if cfg.Machine != "" {
		return cfg.Machine, "config.toml"
	}
	return hostnameSlug(), "hostname"
}

// ResolveEngine returns the absolute path of the engine binary that serves this
// instance. Scheduled jobs need one, and an instance need not contain a binary
// at all: once the engine ships as a release it lives on PATH, and the
// knowledge base holds only knowledge.
//
// Order matches the bin/ shims. A binary inside the instance wins, because in a
// checkout `regesto-install` rebuilds it in place and anything holding that path
// picks up the new build with no re-install. Otherwise the engine running right
// now is by definition a working one, so it is the answer.
func ResolveEngine(cfg *Config) (string, error) {
	inInstance := filepath.Join(cfg.KBRoot, "bin", "regesto")
	if info, err := os.Stat(inInstance); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
		return inInstance, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine which engine serves %s: %w", cfg.KBRoot, err)
	}
	if exe, err = filepath.Abs(exe); err != nil {
		return "", err
	}
	// `go run` builds into a cache directory and deletes it on exit, so a job
	// holding that path would fail on every fire — silently, in a log nobody
	// reads. Refuse instead. A symlink is deliberately NOT resolved: a stable
	// path on PATH is what lets an engine upgrade replace the target without
	// anything else having to be rewritten.
	if d := filepath.Dir(exe); strings.Contains(d, "go-build") || strings.HasPrefix(d, os.TempDir()) {
		return "", fmt.Errorf("this engine is a temporary build at %s, which is deleted on exit.\n"+
			"Install one first — `go build -o %s ./cmd/regesto` in an engine checkout, or a release on PATH — then re-run", exe, inInstance)
	}
	return exe, nil
}

// hostnameSlug turns a host name into a usable short name. Trailing counters are
// stripped because macOS appends and increments them when rejoining networks —
// the same box comes back as "-2", then "-3" — so a raw hostname is not a
// stable identity.
func hostnameSlug() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	h = strings.ToLower(h)
	if i := strings.Index(h, "."); i > 0 {
		h = h[:i]
	}
	h = strings.TrimRight(h, "0123456789")
	h = strings.TrimRight(h, "-")
	if h == "" {
		return "unknown"
	}
	return h
}

func stripComment(line string) string {
	inString := false
	for i, r := range line {
		switch r {
		case '"':
			inString = !inString
		case '#':
			if !inString {
				return line[:i]
			}
		}
	}
	return line
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func parseList(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("expected a [\"a\", \"b\"] list, got %q", value)
	}
	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return nil, nil
	}
	var out []string
	for _, item := range strings.Split(inner, ",") {
		out = append(out, unquote(strings.TrimSpace(item)))
	}
	return out, nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return p
}
