// regesto — single binary with subcommands (decision 0.11). The bin/ shims
// (bin/regesto-search, bin/regesto-index) are thin wrappers over these subcommands so
// hooks and skills get a stable path.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/search"
	"github.com/prof18/regesto/internal/version"
)

const usage = `usage: regesto [--config <path>] <command> [args]

commands:
  search [--json] [--subject S] [--relation R] [--scope SC] [--history] [terms...]
        query knowledge/facts/; superseded hidden unless --history
  index
        regenerate INDEX.md and knowledge/topics/ from knowledge/facts/
  context [--json] [--dir D] [--project P] [--max-bytes N] [--vocabulary]
        compact SessionStart payload: what exists, scoped to the current project
  config [--json]
        print the resolved instance config as key=value lines
  write --source SOURCE [--dir D] --json-input [--json]
        validate and atomically create one fact from a JSON object on stdin
  mcp
        serve local Regesto resources and tools over MCP on stdin/stdout
  install [--dry-run] [--json]
        plan or apply integration skills, instructions, and hook registration
  hook <protocol>
        translate one host hook payload on stdin using host-valid framing
  harvest [--dry-run] [-v]
        capture new native-memory writes into inbox/<agent>@<machine>/
  init [--dir D] [--machine NAME] [--examples] [--force]
        scaffold a new instance: tree, config, adapters, shims, machine identity
  upgrade [--dry-run] [--force]
        refresh this instance's engine-owned files after the engine changed
  version
        which engine this is
  promote [file|-] [--source S] [--name N] [--dry-run]
        chat transcript → facts → archive/chat-exports/ (reads stdin if no file)
  cycle [--dry-run] [--push] [--no-commit]
        the downstream pass: normalise, reconcile, rebuild, commit
  schedule [status|print|install|uninstall]
        run harvest and cycle automatically (launchd on macOS)
  normalize [--dry-run] [--command CMD] [--show-prompt]
        turn inbox captures into canonical facts
  lint [--fix] [--rebuild] [--quiet]
        validate knowledge/facts/ against SCHEMA.md and reconcile contradictions
  project [--json] [--dir D] [--scope] [-v]
        print the canonical project name for a directory
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "regesto:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	global := flag.NewFlagSet("regesto", flag.ContinueOnError)
	configPath := global.String("config", "", "path to config.toml (default: walk up from cwd; REGESTO_CONFIG overrides)")
	global.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	if err := global.Parse(args); err != nil {
		return err
	}
	rest := global.Args()
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("no command given")
	}

	// These two must work without an instance: init runs before one exists, and
	// `version` is the first thing anyone runs to check the install. Every other
	// command resolves the instance first.
	switch rest[0] {
	case "init":
		return runInit(rest[1:])
	case "version", "--version", "-version":
		fmt.Println("regesto", version.Current())
		return nil
	case "hook":
		cfg, err := loadConfig(*configPath)
		if err != nil {
			return failOpenHook(rest[1:], os.Stdout, os.Stderr, err)
		}
		if err := facts.SetConflictPattern(cfg.Section("sync")["conflict_pattern"]); err != nil {
			return failOpenHook(rest[1:], os.Stdout, os.Stderr, err)
		}
		return runHook(cfg, rest[1:])
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
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
