package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/manifest"
	"github.com/prof18/regesto/internal/version"
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

	detected := adapters.Detect()
	if len(detected) > 0 {
		fmt.Printf("  integrations  %s (detected) → config.toml\n", strings.Join(detected, ", "))
	} else {
		fmt.Println("  integrations  none detected — set `integrations` in config.toml once you have one")
	}
	if err := writeIfAbsent(root, config.FileName, instanceConfig(detected)); err != nil {
		return err
	}
	if err := writeIfAbsent(root, ".gitignore", gitignoreTemplate); err != nil {
		return err
	}
	if err := writeIfAbsent(root, ".stignore", stignoreTemplate); err != nil {
		return err
	}

	// The instance-side half of the engine: the schema, the bin/ shims and the
	// adapters. Without them the hook and the skills have nothing to point at
	// and `regesto-install` has nothing to install — an instance is not usable
	// until these are on disk.
	//
	// Their hashes are recorded as they are written, so a later `regesto
	// upgrade` can tell a file this engine wrote from one edited since.
	engine, err := regesto.InstanceFiles()
	if err != nil {
		return err
	}
	m := &manifest.Manifest{Engine: version.Current(), Written: time.Now(), Files: map[string]string{}}
	for _, p := range sortedKeys(engine) {
		if err := writeFileIfAbsent(root, p, engine[p]); err != nil {
			return err
		}
		// Hash what is on disk, not what was offered. Re-running init over an
		// instance whose skill someone edited keeps that edit — recording the
		// engine's hash instead would tell the next upgrade the file was
		// untouched, and it would be overwritten without warning.
		onDisk, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			return err
		}
		m.Files[p] = manifest.Sum(onDisk)
	}
	if err := manifest.Save(root, m); err != nil {
		return err
	}
	fmt.Printf("  engine  %s → %s\n", version.Current(), manifest.FileName)

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

func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// writeFileIfAbsent materialises one engine-owned file, leaving any existing one
// alone. Same rule as unpack, for the flat path-keyed form InstanceFiles returns.
func writeFileIfAbsent(root, rel string, body []byte) error {
	dest := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Stat(dest); err == nil {
		fmt.Printf("  kept    %s (already present)\n", rel)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if regesto.Executable(rel) {
		mode = 0o755
	}
	if err := os.WriteFile(dest, body, mode); err != nil {
		return err
	}
	fmt.Printf("  wrote   %s\n", rel)
	return nil
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

// instanceConfig renders the commented config.toml template. detected is what
// adapters.Detect() found on this machine at init time — the whole point of
// detecting instead of hardcoding a pair is that this list grows the moment
// the engine ships a new adapter, with no line here to update.
func instanceConfig(detected []string) string {
	return `# Regesto instance configuration.
#
# Everything machine- or person-specific lives here, never in the engine. Run
# ` + "`regesto config`" + ` to see what these resolve to.

# Integrations to install and harvest. init detected these as present on this
# machine — edit freely. A configured integration may be absent on a particular
# machine; ` + "`regesto doctor`" + ` reports configured and detected state separately.
# An integration present but not listed remains unmanaged, which is why
# ` + "`regesto upgrade`" + ` mentions newly detected profiles.
` + integrationsLine(detected) + `

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
# commands = "claude -p ;; codex exec --sandbox read-only"

# LaunchAgents do not read shell startup files. The scheduler includes the
# directories of normaliser/notifier commands it can resolve at install time,
# plus stable user, Homebrew and system locations. Add uncommon locations here;
# re-run ` + "`regesto schedule install`" + ` after changing this value or moving a CLI.
# [schedule]
# extra_path = "~/.local/share/mise/shims"

# How the cycle tells you it has stopped working. It runs unattended, so a
# failure is otherwise a line in a log nobody opens — and every hour after the
# first one adds facts that are written but never committed. Notifications fire
# on the transition into failure and back out of it, plus once a day while it
# stays broken; a healthy pass says nothing. macOS uses osascript and Linux
# notify-send unless you name something else here. A custom command is run with
# the title and message as its last two arguments, and the same values in
# $REGESTO_NOTIFY_TITLE and $REGESTO_NOTIFY_MESSAGE.
# [notify]
# on = "off"                    # default is on wherever a notifier exists
# command = "~/bin/my-notifier" # receives: <title> <message>
# renag_hours = "24"            # 0 to report each transition once and never nag

# Exact sources trusted enough to normalise automatically. A private
# channel alone does not grant trust: the human records an exact source approval.
# Unknown or shared surfaces stay raw in the inbox until a human promotes them.
# [trusted_sources]
# "hermes@studio" = "private single-user Telegram channel"

# Source trust rules: exact entries beat trusted-source approvals; trailing-* patterns
# apply after them and before integration defaults.
# Keys are exact <integration>@<machine> IDs or one trailing-* prefix pattern;
# values are exactly supervised or quarantine. Exact rules beat every other rule.
# [source_policies]
# "hermes@studio-public" = "quarantine"
# "hermes-private@studio-*" = "supervised"

# Add a custom host with the portable generic profile. The generated integrations = [...]
# line above is the canonical vocabulary; add the custom ID to that list.
# integrations = ["my-agent"]
# [integrations.my-agent]
# skills_dir = "~/.my-agent/skills"
# instructions_file = "~/.my-agent/AGENTS.md"
# memory_kind = "markdown-glob-v1"
# memory_location = "~/.my-agent/memory"
# trust = "quarantine"
# Diagnose resolved targets without writing: regesto doctor --integration my-agent

# Files never captured from native memory, comma-separated globs. This is the
# control for "not content"; size is only a proxy for it, and a poor one.
# [harvest_exclude]
# codex = "raw_memories.md"

# How your sync client names a conflict copy, as a regular expression. Cycle
# finds those copies, keeps the newer claim and archives the other. The default
# matches Syncthing; set this if you replicate the folder another way, and the
# pattern is cut out of the name to get back to the original file.
#
# Values here are taken literally — backslashes are not escape characters — so
# write the expression exactly as a regex, with single backslashes.
# [sync]
# conflict_pattern = " \(.*conflicted copy.*\)"
`
}

// integrationsLine renders the canonical `integrations = [...]` config line.
// Keep the empty list active so the generated config is explicit.
func integrationsLine(detected []string) string {
	if len(detected) == 0 {
		return "integrations = []   # no known integration detected — add yours here"
	}
	quoted := make([]string, len(detected))
	for i, a := range detected {
		quoted[i] = `"` + a + `"`
	}
	return "integrations = [" + strings.Join(quoted, ", ") + "]"
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
