package main

// regesto integrations — manage the `integrations` list in config.toml and
// nothing else. Adding an integration to a live instance used to mean
// hand-editing a strict TOML list with no guidance toward the follow-up
// steps; this command is that edit, done surgically (one line replaced, every
// comment preserved) and paired with the next commands to run.
//
// It deliberately installs nothing: installing touches the agent hosts, and
// `regesto install` already knows how. upgrade never writes config.toml
// because it is not an explicit user action; this command is, which is what
// licenses the write.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
)

func runIntegrations(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("integrations", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: regesto integrations [add|remove] [id...]\n\n"+
			"Choose which agents Regesto connects to. This only edits config.toml;\n"+
			"run `regesto install` afterwards.\n\n"+
			"  add <id...>     Connect agents (also the default when you just pass ids).\n"+
			"  remove <id...>  Disconnect agents and delete their settings in config.toml.\n"+
			"  (no ids)        Pick from a checklist.\n\n"+
			"Built-in ids: "+strings.Join(adapters.KnownAgents(), ", ")+"\n"+
			"Other agents are added by hand in config.toml.")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	sub := "add"
	if len(rest) > 0 && (rest[0] == "add" || rest[0] == "remove") {
		sub = rest[0]
		rest = rest[1:]
	}
	switch sub {
	case "add":
		return addIntegrations(cfg, rest)
	case "remove":
		return removeIntegrations(cfg, rest)
	default:
		return fmt.Errorf("integrations: unknown subcommand %q", sub)
	}
}

// addIntegrations appends the given ids, or opens the interactive setter when
// none are given. Ids already configured are a no-op; ids must be known
// profiles or already configured.
func addIntegrations(cfg *config.Config, ids []string) error {
	current := cfg.IntegrationIDs()
	known := adapters.KnownAgents()

	if len(ids) == 0 {
		return setIntegrationsInteractive(cfg, current, known)
	}

	var unknown []string
	for _, id := range ids {
		if !slices.Contains(known, id) && !slices.Contains(current, id) {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown agent %s (built-in: %s); add other agents by hand, see [integrations.my-agent] in config.toml",
			strings.Join(quoteAll(unknown), ", "), strings.Join(known, ", "))
	}
	next := slices.Clone(current)
	var added []string
	for _, id := range ids {
		if !slices.Contains(next, id) {
			next = append(next, id)
			added = append(added, id)
		}
	}
	if len(added) == 0 {
		fmt.Printf("%s already connected. Nothing to do.\n", integrationNames(ids))
		return nil
	}
	if _, err := saveIntegrations(cfg, next); err != nil {
		return err
	}
	fmt.Printf("Connected %s in config.toml.\n", integrationNames(added))
	printIntegrationNextSteps()
	return nil
}

// removeIntegrations removes the given ids, or opens the interactive setter
// when none are given. Removing an id also drops its [integrations.<id>]
// override table: a table naming an unlisted integration makes every command
// fail to load the config.
func removeIntegrations(cfg *config.Config, ids []string) error {
	if len(ids) == 0 {
		return setIntegrationsInteractive(cfg, cfg.IntegrationIDs(), adapters.KnownAgents())
	}
	current := cfg.IntegrationIDs()
	known := adapters.KnownAgents()
	var unknown []string
	for _, id := range ids {
		if !slices.Contains(known, id) && !slices.Contains(current, id) {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown agent %s (built-in: %s); remove other agents by hand in config.toml",
			strings.Join(quoteAll(unknown), ", "), strings.Join(known, ", "))
	}
	next := make([]string, 0, len(current))
	var removed []string
	for _, id := range current {
		if slices.Contains(ids, id) {
			removed = append(removed, id)
			continue
		}
		next = append(next, id)
	}
	if len(removed) == 0 {
		fmt.Printf("%s not connected. Nothing to do.\n", integrationNames(ids))
		return nil
	}
	dropped, err := saveIntegrations(cfg, next)
	if err != nil {
		return err
	}
	fmt.Printf("Disconnected %s in config.toml.\n", integrationNames(removed))
	printDroppedOverrides(dropped)
	printIntegrationNextSteps()
	return nil
}

// setIntegrationsInteractive is the no-ids path: the init picker seeded with
// the configured list plus detected-but-unconfigured integrations, pre-checked
// = exactly what is configured today. Confirming rewrites the whole list, so
// unchecking removes an integration from the managed set — and its override
// table, if any, goes with it (saveIntegrations).
func setIntegrationsInteractive(cfg *config.Config, current, known []string) error {
	items := slices.Clone(current)
	for _, id := range adapters.Detect() {
		if !slices.Contains(items, id) {
			items = append(items, id)
		}
	}
	if len(items) == 0 {
		fmt.Println("No agents connected or found on this machine. Pass an id once one is installed.")
		return nil
	}
	p := newPicker(items)
	p.title = "Agents Regesto connects to. Checked ones stay connected:"
	for i, id := range items {
		p.checked[i] = slices.Contains(current, id)
	}
	picked, err := runPicker(os.Stdin, os.Stdout, p)
	switch {
	case err == nil:
	case errors.Is(err, errAborted):
		return fmt.Errorf("%w; config.toml unchanged", errAborted)
	case errors.Is(err, errNotATerminal):
		return fmt.Errorf("no agent ids given; pass ids (built-in: %s), e.g. regesto integrations add codex", strings.Join(known, ", "))
	default:
		return err
	}
	if slices.Equal(picked, current) {
		fmt.Println("No changes.")
		return nil
	}
	dropped, err := saveIntegrations(cfg, picked)
	if err != nil {
		return err
	}
	if len(picked) == 0 {
		fmt.Println("No agents connected now; config.toml updated.")
	} else {
		fmt.Printf("Connected agents are now %s (config.toml updated).\n", integrationNames(picked))
	}
	printDroppedOverrides(dropped)
	printIntegrationNextSteps()
	return nil
}

// saveIntegrations rewrites the active `integrations` line and, for every id
// that leaves the list, deletes its [integrations.<id>] override table —
// dropping the id but keeping its table would orphan a section that
// adapters.Resolve rejects, taking every command down with the config. It
// returns the ids whose tables were dropped.
//
// The edited body must pass the same gates a real load goes through —
// config.Load alone accepts bodies adapters.Resolve rejects — before it is
// allowed to replace the live file.
func saveIntegrations(cfg *config.Config, list []string) ([]string, error) {
	var removed []string
	for _, id := range cfg.IntegrationIDs() {
		if !slices.Contains(list, id) {
			removed = append(removed, id)
		}
	}
	body, err := os.ReadFile(cfg.Path)
	if err != nil {
		return nil, err
	}
	edited := string(body)
	var dropped []string
	if len(removed) > 0 {
		edited, dropped = config.RemoveIntegrationSections(edited, removed...)
	}
	edited = config.RewriteIntegrations(edited, list)

	mode := os.FileMode(0o644)
	if info, err := os.Stat(cfg.Path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(cfg.Path), "."+config.FileName+"-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(edited); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return nil, err
	}
	parsed, err := config.Load(tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("could not update config.toml safely, nothing written: %w", err)
	}
	if _, err := adapters.Resolve(parsed); err != nil {
		return nil, fmt.Errorf("could not update config.toml safely, nothing written: %w", err)
	}
	if !slices.Equal(parsed.Integrations, list) {
		return nil, fmt.Errorf("could not update config.toml safely, nothing written: the integrations line did not come out as expected")
	}
	if err := os.Rename(tmp.Name(), cfg.Path); err != nil {
		return nil, err
	}
	return dropped, nil
}

func printIntegrationNextSteps() {
	fmt.Println("\nNext steps")
	fmt.Println("  1. regesto install --dry-run   Preview the changes to your agents")
	fmt.Println("  2. regesto install             Apply them")
}

func printDroppedOverrides(dropped []string) {
	if len(dropped) == 0 {
		return
	}
	sections := make([]string, len(dropped))
	for i, id := range dropped {
		sections[i] = "[integrations." + id + "]"
	}
	fmt.Printf("Also deleted %s from config.toml.\n", strings.Join(sections, ", "))
}

func quoteAll(ids []string) []string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	return quoted
}
