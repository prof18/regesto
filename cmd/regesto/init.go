package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
	setUsage(fs, "regesto init [options]", "Create a new knowledge base.")
	dir := fs.String("dir", "", "folder to create it in (default: current folder)")
	machine := fs.String("machine", "", "short name for this machine (default: from the hostname)")
	force := fs.Bool("force", false, "add missing files to an existing knowledge base, keeping existing ones")
	examples := fs.Bool("examples", false, "add a few example facts")
	integrations := fs.String("integrations", "", "comma-separated agent ids to connect, instead of choosing interactively; \",\" for none")
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
	configExists := false
	if _, err := os.Stat(configPath); err == nil {
		configExists = true
		if !*force {
			return fmt.Errorf("%s already exists, so this is already a knowledge base; pass --force to add any missing files", tildePath(configPath))
		}
		if *integrations != "" {
			// An explicit flag that cannot take effect must not be swallowed.
			return fmt.Errorf("--integrations has no effect with --force, because the existing config.toml is kept; use `regesto integrations add <id>` instead")
		}
	}

	printInitWelcome(root, configExists)

	// Choose the integrations before anything is written: a user forbidden
	// from a tool must be able to opt out here, and aborting the picker must
	// leave the directory untouched. The picker appears when stdin and stdout
	// are a terminal; anything else — pipes, CI — gets today's non-interactive
	// behavior. Automation that allocates a pty (expect, Ansible pty: yes)
	// reads as a terminal, so it must pass --integrations, which skips the
	// picker entirely.
	//
	// Over an existing config (--force) the config file is kept, so there is
	// nothing to choose — prompting would discard the user's answer.
	detected := adapters.Detect()
	chosen := detected
	note := "(found on this machine)"
	interactive := false
	if !configExists {
		if *integrations != "" {
			var err error
			chosen, err = parseIntegrationsFlag(*integrations)
			if err != nil {
				return err
			}
			note = ""
		} else if len(detected) > 0 {
			picked, err := selectIntegrations(os.Stdin, os.Stdout, detected)
			switch {
			case err == nil:
				chosen = picked
				note = ""
				interactive = true
			case errors.Is(err, errAborted):
				return fmt.Errorf("%w — nothing written", errAborted)
			case errors.Is(err, errNotATerminal):
				// Scripted context: configure everything detected, as before.
			default:
				return err
			}
		}
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
	}
	// .state is per-machine and deliberately excluded from both git and
	// Syncthing, so it gets no .gitkeep.
	if err := os.MkdirAll(filepath.Join(root, ".state"), 0o755); err != nil {
		return err
	}

	name := strings.TrimSpace(*machine)
	if name == "" {
		// Reuse the same resolution the rest of the tool uses, so init cannot
		// disagree with it.
		tmp := &config.Config{KBRoot: root}
		name, _ = config.ResolveMachineFor(tmp)
	}
	statePath := filepath.Join(root, ".state", config.MachineFile)
	if _, err := os.Stat(statePath); err != nil {
		if err := os.WriteFile(statePath, []byte(name+"\n"), 0o644); err != nil {
			return err
		}
	}
	kept := 0
	for _, f := range []struct{ name, body string }{
		{config.FileName, instanceConfig(chosen)},
		{".gitignore", gitignoreTemplate},
		{".stignore", stignoreTemplate},
	} {
		wrote, err := writeIfAbsent(root, f.name, f.body)
		if err != nil {
			return err
		}
		if !wrote {
			kept++
		}
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
		wrote, err := writeFileIfAbsent(root, p, engine[p])
		if err != nil {
			return err
		}
		if !wrote {
			kept++
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

	if *examples {
		src, err := exampleFacts()
		if err != nil {
			return err
		}
		n, err := unpack(filepath.Join(root, "knowledge", "facts"), src)
		if err != nil {
			return err
		}
		kept += n
	}

	imported := 0
	if interactive && len(chosen) > 0 {
		fmt.Println()
		imported = offerMemoryImport(os.Stdin, os.Stdout, configPath)
	}

	printInitSummary(root, name, chosen, detected, note, configExists, kept, imported)
	return nil
}

// printInitWelcome says what init is about to do before anything is asked, so
// the agent picker does not appear without context.
func printInitWelcome(root string, configExists bool) {
	if configExists {
		fmt.Printf("Adding missing files to the knowledge base in %s\n\n", tildePath(root))
		return
	}
	fmt.Printf("Welcome to Regesto! Creating a new knowledge base in %s\n\n", tildePath(root))
	fmt.Println("Your agents' decisions, preferences and facts are kept here as plain Markdown")
	fmt.Println("files, and each agent can search them and add to them.")
	fmt.Println()
}

// printInitSummary is what a person reads right after init: where the
// knowledge base is, what it is set up for, and the few commands that make it
// useful. The individual files are an implementation detail and are not listed.
func printInitSummary(root, machine string, chosen, detected []string, note string, configExists bool, kept, imported int) {
	fmt.Printf("Done. Knowledge base ready in %s\n\n", tildePath(root))
	fmt.Printf("  Machine name  %s\n", machine)
	// When config.toml is kept, the agent list was not written by this run.
	if !configExists {
		switch {
		case len(chosen) > 0:
			fmt.Println(strings.TrimRight("  Agents        "+integrationNames(chosen)+" "+note, " "))
		case len(detected) > 0:
			fmt.Println("  Agents        none selected — add them later with `regesto integrations add <id>`")
		default:
			fmt.Println("  Agents        none found on this machine — add them later with `regesto integrations add <id>`")
		}
	}
	if imported > 0 {
		fmt.Printf("  Inbox         %s imported from existing agent memory\n", plural(imported, "note"))
	}
	fmt.Printf("  Settings      %s\n", tildePath(filepath.Join(root, config.FileName)))
	if kept > 0 {
		fmt.Printf("  Unchanged     %s that already existed\n", plural(kept, "file"))
	}

	fmt.Println("\nNext steps")
	step := 1
	if cwd, err := os.Getwd(); err != nil || filepath.Clean(cwd) != root {
		fmt.Printf("  %d. cd %s\n", step, tildePath(root))
		step++
	}
	fmt.Printf("  %d. regesto install --dry-run   Preview what will be added to your agents\n", step)
	fmt.Printf("  %d. regesto install             Add Regesto's skills, instructions and hooks\n", step+1)
	fmt.Printf("  %d. regesto schedule install    Collect and process knowledge automatically\n", step+2)
	fmt.Printf("  %d. Start a new agent session in a project; its facts load automatically.\n", step+3)
	fmt.Println("\nOptional: run `git init` here to keep a history of changes.")
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
func unpack(root string, src fs.FS) (int, error) {
	kept := 0
	err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." {
			return err
		}
		dest := filepath.Join(root, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		if _, err := os.Stat(dest); err == nil {
			kept++
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
		return os.WriteFile(dest, body, mode)
	})
	return kept, err
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
// It reports whether it wrote the file.
func writeFileIfAbsent(root, rel string, body []byte) (bool, error) {
	dest := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Stat(dest); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, err
	}
	mode := os.FileMode(0o644)
	if regesto.Executable(rel) {
		mode = 0o755
	}
	if err := os.WriteFile(dest, body, mode); err != nil {
		return false, err
	}
	return true, nil
}

// writeIfAbsent writes a template file unless one exists, reporting whether it
// wrote.
func writeIfAbsent(root, name, body string) (bool, error) {
	path := filepath.Join(root, name)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// instanceConfig renders the config.toml template. chosen is what the picker,
// --integrations or plain detection settled on — the ids that go into the
// active integrations line. Comments here are read by people opening the file
// for the first time: say what a setting does, not why it is designed that way.
func instanceConfig(chosen []string) string {
	return `# Regesto settings for this knowledge base.
# See the values in effect with ` + "`regesto config`" + `; check setup with ` + "`regesto doctor`" + `.

# Agents Regesto connects to. Change with ` + "`regesto integrations add <id>`" + ` or ` + "`remove <id>`" + `.
` + integrationsLine(chosen) + `

# Everything below is optional, and the defaults work for most people.
# Remove the leading # to turn a setting on.

# Only if a project gets the wrong name. Projects are named after their git
# remote; map a folder name here, e.g. for a repo with no remote.
# [projects]
# "my-app-fork" = "my-app"

# Only if this folder is synced across several machines: the one machine that
# processes the inbox. Without it, every machine does.
# [roles]
# lint = "my-macbook"

# Only to choose the agent commands that turn notes into facts. By default:
# Claude Code, then Codex; if neither is connected, your other agents. Separate
# commands with ;; and put {prompt} where a command takes the prompt as text.
# [normalize]
# commands = "my-agent --print ;; claude -p"

# Rarely needed. Only if a background job can't find a command that works in
# your terminal (jobs don't load your shell profile): add its folder here, then
# run ` + "`regesto schedule install`" + ` again.
# [schedule]
# extra_path = "~/.local/share/mise/shims"

# Only if you want an alert when background processing fails and recovers
# (agents see failures anyway). The command gets a title and a message.
# [notify]
# command = "/absolute/path/to/notifier"
# on = "off"           # pause alerts
# renag_hours = "24"   # repeat while still failing; 0 = never repeat

# Only to override an agent's default: "supervised" notes become facts
# automatically, "quarantine" ones wait in the inbox for you. Keys are
# agent@machine; a trailing * matches several machines.
# [source_policies]
# "hermes@my-macbook" = "supervised"
# "hermes@work-laptop-*" = "quarantine"

# Only for an agent without built-in support. Add its id to integrations above
# too, then check it with ` + "`regesto doctor --integration my-agent`" + `.
# Agents that only use Regesto through MCP need no entry.
# [integrations.my-agent]
# profile = "generic"
# skills_dir = "~/.my-agent/skills"
# instructions_file = "~/.my-agent/AGENTS.md"   # must already exist
# memory_kind = "markdown-glob-v1"
# memory_location = "~/.my-agent/memory"
# trust = "quarantine"

# Only if some memory files shouldn't be collected: comma-separated patterns
# per agent.
# [harvest_exclude]
# codex = "raw_memories.md"

# Only if you sync with something other than Syncthing: how your tool names
# conflict copies, as a regular expression (write each backslash once). This
# example matches Dropbox.
# [sync]
# conflict_pattern = " \(.*conflicted copy.*\)"
`
}

// integrationsLine renders the canonical `integrations = [...]` config line.
// Keep the empty list active so the generated config is explicit.
func integrationsLine(chosen []string) string {
	if len(chosen) == 0 {
		return "integrations = []"
	}
	quoted := make([]string, len(chosen))
	for i, a := range chosen {
		quoted[i] = `"` + a + `"`
	}
	return "integrations = [" + strings.Join(quoted, ", ") + "]"
}

// parseIntegrationsFlag parses the --integrations value: comma-separated ids,
// validated against the engine's known profiles so a typo cannot silently
// configure an integration that will never install or harvest.
func parseIntegrationsFlag(raw string) ([]string, error) {
	known := adapters.KnownAgents()
	var out []string
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if !slices.Contains(known, id) {
			return nil, fmt.Errorf("unknown integration %q — known: %s", id, strings.Join(known, ", "))
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
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
