// regesto — single binary with subcommands (decision 0.11). The bin/ shims
// (bin/regesto-search, bin/regesto-index) are thin wrappers over these subcommands so
// hooks and skills get a stable path.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/search"
	"github.com/prof18/regesto/internal/version"
)

const usage = `Regesto — a shared knowledge base for your agents

Usage: regesto [--config <path>] <command> [args]

Get started
  init          Create a knowledge base: regesto init --dir ~/regesto-kb
  integrations  Choose which agents Regesto connects to.
  install       Add Regesto's skills, instructions and hooks to those agents.
  doctor        Check the setup and say what to fix.
  schedule      Run collection and processing in the background.

Find and add knowledge
  search        Find facts by words, subject, relation or project.
  write         Save a fact given as JSON (mostly used by agents).
  promote       Pull facts out of a chat transcript.
  project       Show the project name Regesto uses for a folder.

Keep it tidy (the background jobs run these for you)
  harvest       Collect new agent memories into the inbox.
  normalize     Turn inbox notes into facts.
  lint          Check facts and resolve contradictions.
  index         Rebuild INDEX.md and the topic pages.
  cycle         All of the above, then commit to git.

Maintenance
  upgrade       Update Regesto's files after installing a new version.
  config        Show the settings in effect.
  version       Show the Regesto version.

Used by agents
  context       Print the summary an agent sees at session start.
  hook          Handle an agent hook (not run by hand).
  mcp           Run the MCP server.

Run regesto <command> --help for a command's options.
Commands find config.toml by looking in the current folder and its parents;
use --config <path> before the command to pick another knowledge base.
`

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "regesto:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	global := flag.NewFlagSet("regesto", flag.ContinueOnError)
	configPath := global.String("config", "", "path to config.toml (default: walk up from cwd; REGESTO_CONFIG overrides)")
	global.Usage = func() { fmt.Fprint(os.Stdout, usage) }
	if err := global.Parse(args); err != nil {
		return err
	}
	rest := global.Args()
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("no command given; see the list above")
	}

	// Setup, version, and help must also work before an instance exists.
	switch rest[0] {
	case "help":
		fmt.Fprint(os.Stdout, usage)
		return nil
	case "init":
		return runInit(rest[1:])
	case "version", "--version", "-version":
		fmt.Println("regesto", version.Current())
		return nil
	case "hook":
		if len(rest) == 2 && (rest[1] == "--help" || rest[1] == "-h" || rest[1] == "-help") {
			fmt.Println("Usage: regesto hook <protocol>\n\nCalled by an agent's hook to load the knowledge base into a session.\n`regesto install` sets this up; you don't run it by hand.\n\nProtocols: antigravity-pre-invocation-v1, claude-session-start-v1, copilot-session-start-v1, hermes-pre-llm-v1, opencode-chat-message-v1")
			return nil
		}
		cfg, err := loadConfig(*configPath)
		if err != nil {
			return failOpenHook(rest[1:], os.Stdout, os.Stderr, err)
		}
		if err := facts.SetConflictPattern(cfg.Section("sync")["conflict_pattern"]); err != nil {
			return failOpenHook(rest[1:], os.Stdout, os.Stderr, err)
		}
		return runHook(cfg, rest[1:])
	}

	var cfg *config.Config
	var err error
	if len(rest) == 2 && (rest[1] == "--help" || rest[1] == "-h" || rest[1] == "-help") {
		switch rest[0] {
		case "index", "mcp":
			descriptions := map[string]string{
				"index": "Rebuild INDEX.md and the topic pages from the facts.",
				"mcp":   "Run the MCP server on stdin/stdout, for agents that use MCP.",
			}
			fmt.Printf("Usage: regesto %s\n\n%s\nThis command has no options.\n", rest[0], descriptions[rest[0]])
			return nil
		}
		cfg = &config.Config{}
	} else {
		cfg, err = loadConfig(*configPath)
		if err != nil {
			return err
		}
	}
	// Applied once, before any command can load a fact: the loader skips
	// conflict copies, so it and the conflict finder have to agree on what one
	// looks like.
	if err := facts.SetConflictPattern(cfg.Section("sync")["conflict_pattern"]); err != nil {
		return err
	}

	switch rest[0] {
	case "search":
		return runSearch(cfg, rest[1:])
	case "index":
		return runIndex(cfg)
	case "context":
		return runContext(cfg, rest[1:])
	case "config":
		return runShowConfig(cfg, rest[1:])
	case "write":
		return runWrite(cfg, rest[1:])
	case "mcp":
		return runMCP(cfg, rest[1:])
	case "install":
		return runInstall(cfg, rest[1:])
	case "integrations":
		return runIntegrations(cfg, rest[1:])
	case "doctor":
		return runDoctor(cfg, rest[1:])
	case "harvest":
		return runHarvest(cfg, rest[1:])
	case "promote":
		return runPromote(cfg, rest[1:])
	case "cycle":
		return runCycle(cfg, rest[1:])
	case "schedule":
		return runSchedule(cfg, rest[1:])
	case "normalize":
		return runNormalize(cfg, rest[1:])
	case "lint":
		return runLint(cfg, rest[1:])
	case "project":
		return runProject(cfg, rest[1:])
	case "upgrade":
		return runUpgrade(cfg, rest[1:])
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q; see the list above", rest[0])
	}
}

func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		path, err = config.Find(cwd)
		if err != nil {
			return nil, err
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if _, err := adapters.Resolve(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func runSearch(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	setUsage(fs, "regesto search [options] [words...]", "Find facts. Every option and word given must match.")
	subject := fs.String("subject", "", "only facts with exactly this subject")
	relation := fs.String("relation", "", "only facts with exactly this relation")
	scope := fs.String("scope", "", "only global facts, or one project's (project:<name> or just <name>)")
	history := fs.Bool("history", false, "include facts that have been replaced")
	jsonOutput := fs.Bool("json", false, "print the results as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}
	results := search.Run(all, search.Query{
		Subject:  *subject,
		Relation: *relation,
		Scope:    *scope,
		Terms:    fs.Args(),
		History:  *history,
	})
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(jsonSearchResponse{SchemaVersion: jsonSchemaVersion, Results: jsonFacts(results)})
	}
	for _, f := range results {
		fmt.Println(search.FormatLine(f))
	}
	return nil
}
