package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/config"
)

// runInit scaffolds a new instance: the tree, a commented config, the ignore
// files, and this machine's identity.
//
// This is the spec PLAN 4.a asks for — the manual steps of Phases 0 and 1,
// written down as a script. It creates an *instance*; the engine is whatever
// binary is running it.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	dir := fs.String("dir", "", "where to create the instance (default: cwd)")
	machine := fs.String("machine", "", "short name for this machine (default: derived from the hostname)")
	force := fs.Bool("force", false, "write into a directory that already has a config.toml")
	examples := fs.Bool("examples", false, "populate knowledge/facts/ with the example facts")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root := *dir
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		root = cwd
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	configPath := filepath.Join(root, config.FileName)
	if _, err := os.Stat(configPath); err == nil && !*force {
		return fmt.Errorf("%s already exists — this looks like an instance already; pass --force to write anyway", configPath)
	}

	// The tree. .gitkeep in every directory that would otherwise be empty,
	// because git does not track directories.
	dirs := []string{
		"knowledge/facts/global",
		"knowledge/facts/projects",
		"knowledge/topics",
		"inbox",
		"archive/inbox",
		"archive/chat-exports",
		"bin",
		"docs",
	}
	for _, d := range dirs {
		full := filepath.Join(root, d)
		if err := os.MkdirAll(full, 0o755); err != nil {
			return err
		}
		keep := filepath.Join(full, ".gitkeep")
		if _, err := os.Stat(keep); err != nil {
			if err := os.WriteFile(keep, nil, 0o644); err != nil {
				return err
			}
		}
		fmt.Printf("  dir     %s\n", d)
	}
	// .state is per-machine and deliberately excluded from both git and
	// Syncthing, so it gets no .gitkeep.
	if err := os.MkdirAll(filepath.Join(root, ".state"), 0o755); err != nil {
		return err
	}

	name := strings.TrimSpace(*machine)
	derived := ""
	if name == "" {
		// Reuse the same resolution the rest of the tool uses, so init cannot
		// disagree with it.
		tmp := &config.Config{KBRoot: root}
		name, derived = config.ResolveMachineFor(tmp)
	}
	statePath := filepath.Join(root, ".state", config.MachineFile)
	if _, err := os.Stat(statePath); err != nil {
		if err := os.WriteFile(statePath, []byte(name+"\n"), 0o644); err != nil {
			return err
		}
	}
	origin := "given"
	if derived != "" {
		origin = "derived from " + derived
	}
	fmt.Printf("  machine %s (%s) → .state/machine\n", name, origin)

	if err := writeIfAbsent(root, config.FileName, instanceConfig()); err != nil {
		return err
	}
	if err := writeIfAbsent(root, ".gitignore", gitignoreTemplate); err != nil {
		return err
	}
	if err := writeIfAbsent(root, ".stignore", stignoreTemplate); err != nil {
		return err
	}
	if err := writeIfAbsent(root, "SCHEMA.md", regesto.Schema); err != nil {
		return err
	}

	// The instance-side half of the engine. Without it the shims, the hook and
	// the skills have nothing to point at, and `regesto-install` has nothing to
	// install — an instance is not usable until these are on disk.
	if err := unpack(root, regesto.Shims); err != nil {
		return err
	}
	if err := unpack(root, regesto.Adapters); err != nil {
		return err
	}

	if *examples {
		src, err := exampleFacts()
		if err != nil {
			return err
		}
		if err := unpack(filepath.Join(root, "knowledge", "facts"), src); err != nil {
			return err
		}
	}

	fmt.Println()
	fmt.Println("instance ready at", root)
	fmt.Println()
	fmt.Println("next:")
	fmt.Println("  1. git init                       # optional; the markdown is the knowledge, git is convenience")
	fmt.Println("  2. bin/regesto-install --dry-run  # then without --dry-run: skills, hook, instructions")
	fmt.Println("  3. bin/regesto schedule install   # harvest every 15 min, cycle hourly")
	fmt.Println("  4. open a session in a project — its facts should appear without being asked for")
	return nil
}

// exampleFacts is the embedded examples/facts tree with its prefix stripped, so
// it unpacks straight onto knowledge/facts/ and keeps its global/ and projects/
// split.
func exampleFacts() (fs.FS, error) {
	return fs.Sub(regesto.Examples, "examples/facts")
}

// unpack materialises an embedded tree under root, leaving any file that is
// already there alone. Scripts land executable; nothing else does.
//
// Existing files are never overwritten because init is a scaffold, not an
// upgrade: re-running it on a live instance must not silently revert a local
// edit to a skill. Refreshing an instance after an engine upgrade is a separate
// job, and does not exist yet.
func unpack(root string, src fs.FS) error {
	return fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." {
			return err
		}
		dest := filepath.Join(root, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		if _, err := os.Stat(dest); err == nil {
			fmt.Printf("  kept    %s (already present)\n", p)
			return nil
		}
		body, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(p, ".sh") || strings.HasPrefix(p, "bin/") {
			mode = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, body, mode); err != nil {
			return err
		}
		fmt.Printf("  wrote   %s\n", p)
		return nil
	})
}

func writeIfAbsent(root, name, body string) error {
	path := filepath.Join(root, name)
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("  kept    %s (already present)\n", name)
		return nil
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Printf("  wrote   %s\n", name)
	return nil
}

func instanceConfig() string {
	return `# Regesto instance configuration.
#
# Everything machine- or person-specific lives here, never in the engine. Run
# ` + "`regesto config`" + ` to see what these resolve to.

# Agents to install adapters for and harvest from. An agent that is not present
# on this machine is skipped rather than being an error.
agents = ["claude", "codex"]

# Machine identity is NOT set here. This file sits at the KB root, which a sync
# client replicates, so a value here would be identical on every machine — while
# inbox/<agent>@<machine>/, .state/<machine>/ and a fact's source: all have to
# differ. It resolves per machine instead: $REGESTO_MACHINE, then .state/machine
# (written by ` + "`regesto init`" + `), then a hostname slug.

# Canonical project names. The name is derived from the git remote's basename,
# which every clone of a project shares; this table is only needed for
# remote-less repos and for two different projects whose basenames collide.
# [projects]
# "my-app-fork" = "my-app"

# Which machine runs the downstream pass. Capture happens everywhere; deciding
# what becomes a fact happens in one place, or two machines mint competing
# vocabulary for the same claim. Unset means "this machine".
# [roles]
# lint = "studio"

# The agent invocation that turns raw captures into candidate facts. It reads a
# prompt on stdin and prints the answer, so any command of that shape works.
# [normalize]
# command = "claude -p"

# Sources trusted enough to normalise automatically. Trust follows the channel,
# not the agent: a private single-user channel is you; anything a third party can
# reach is not, and stays raw in the inbox until a human promotes it.
# [trusted_sources]
# "hermes@studio" = "private single-user Telegram channel"

# Install locations, if this machine differs from the vendor defaults
# (~/.claude/skills, ~/.claude/CLAUDE.md, ~/.codex/skills, ~/.codex/AGENTS.md).
# Symlinked targets are handled automatically.
# [skills_dirs]
# claude = "~/.agents/skills"
# [instructions]
# claude = "~/.dotfiles/AGENTS.md"

# Files never captured from native memory, comma-separated globs. This is the
# control for "not content"; size is only a proxy for it, and a poor one.
# [harvest_exclude]
# codex = "raw_memories.md"
`
}

const gitignoreTemplate = `# macOS noise
.DS_Store

# Per-machine harvest baselines and identity. NEVER commit and NEVER sync: each
# machine diffs its native memory against its own last snapshot. If .state/ is
# shared, machines overwrite each other's baselines and captures are lost
# silently. Also listed in .stignore. Do not "tidy up" either entry.
.state/

# Sync client artifacts
.stversions/
*.sync-conflict-*

# Built binary
/bin/regesto

# Obsidian per-session UI state, if this is opened as a vault
.obsidian/workspace.json
.obsidian/workspace-mobile.json
`

const stignoreTemplate = `// Sync ignore file. Comments use //.
.DS_Store
(?d).stversions
// Per-machine baselines and identity — see .gitignore for why this must never
// sync. Sharing it loses captures silently.
.state
// Load-bearing: git is intended to live on one machine. A machine with no .git
// cannot corrupt it, and a second machine running 'git status' would otherwise
// become a second writer.
.git
`
