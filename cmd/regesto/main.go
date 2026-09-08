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

Find and record knowledge
  search       Find facts by subject, relation, scope, or search terms.
  write        Validate and save a fact supplied as JSON on stdin.
  promote      Extract facts from a chat transcript (file or stdin).
  project      Show the project name for a directory.

Maintain your knowledge base
  harvest      Collect new agent memories into the inbox.
  normalize    Turn inbox captures into facts.
  lint         Check facts and reconcile contradictions.
  index        Rebuild INDEX.md and topic pages from facts.
  cycle        Normalize, reconcile, rebuild, and commit changes.

Set up and troubleshoot
  init         Create a knowledge base. Start with: regesto init --dir <path>
  install      Set up agent skills, instructions, and hooks.
  doctor       Check integrations and show what needs attention.
  upgrade      Refresh the files managed by Regesto in this instance.
  schedule     Manage scheduled runs: status, print, install, uninstall.
  config       Show the resolved configuration.
  version      Show the engine version.

Agent interfaces
  context      Produce knowledge-base context for an agent session.
  hook         Respond to a host hook request on stdin.
  mcp          Serve local resources and tools over MCP on stdin/stdout.

Help
  regesto <command> --help     Show command options.
  regesto help                Show this overview.

Examples
  regesto search --scope aurora caching
  regesto doctor
  regesto install --dry-run

Use --config <path> before the command to select a knowledge base.
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
		return fmt.Errorf("no command given")
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
			fmt.Println("Usage: regesto hook <protocol>\n\nRead a host hook request from stdin.\nProtocols: claude-session-start-v1, hermes-pre-llm-v1")
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
			fmt.Printf("Usage: regesto %s\n\nThis command takes no options.\n", rest[0])
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
		return fmt.Errorf("unknown command %q", rest[0])
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
	subject := fs.String("subject", "", "exact subject match")
	relation := fs.String("relation", "", "exact relation match")
	scope := fs.String("scope", "", "global, project:<name>, or bare project name")
	history := fs.Bool("history", false, "include status: superseded claims")
	jsonOutput := fs.Bool("json", false, "print matching facts as JSON")
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
