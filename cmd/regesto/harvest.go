package main

import (
	"flag"
	"fmt"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/harvest"
)

func runHarvest(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("harvest", flag.ContinueOnError)
	setUsage(fs, "regesto harvest [options] [agent...]", "Collect new or changed agent memory files into the inbox.\n"+
		"The first run only records what already exists; pass --backfill to import it too.")
	dry := fs.Bool("dry-run", false, "show what would be collected without writing anything")
	verbose := fs.Bool("v", false, "also list the files that were skipped, and why")
	backfill := fs.Bool("backfill", false, "also import memory that already existed, once per agent (limit it by naming agents)")
	if err := fs.Parse(flagsFirst(fs, args)); err != nil {
		return err
	}
	if fs.NArg() > 0 && !*backfill {
		return fmt.Errorf("agent names only apply with --backfill, e.g. regesto harvest --backfill %s", fs.Arg(0))
	}

	results, err := harvest.RunWithOptions(cfg, harvest.Options{DryRun: *dry, Backfill: *backfill, BackfillAgents: fs.Args()})
	if err != nil {
		return err
	}
	printHarvestResults(results, *dry, *verbose, *backfill)
	return nil
}

// printHarvestResults reports one pass: a line per memory location, then a total.
func printHarvestResults(results []harvest.Result, dry, verbose, backfill bool) {
	sources := map[string]int{}
	for _, r := range results {
		sources[r.Agent]++
	}
	total := 0
	for _, r := range results {
		label := adapters.DisplayName(r.Agent)
		// The source id only helps tell apart two memory locations of one agent.
		if r.SourceID != "" && sources[r.Agent] > 1 {
			label += " (" + r.SourceID + ")"
		}
		if r.Note != "" {
			fmt.Printf("%s: %s\n", label, r.Note)
		}
		for _, c := range r.Captured {
			fmt.Printf("%s: collected %s\n", label, c)
		}
		if verbose {
			for _, s := range r.Skipped {
				fmt.Printf("%s: skipped %s\n", label, s)
			}
		}
		if r.Note == "" && len(r.Captured) == 0 {
			fmt.Printf("%s: nothing new (scanned %s)\n", label, plural(r.Scanned, "file"))
		}
		total += len(r.Captured)
	}
	switch {
	case dry:
		fmt.Printf("\nDry run: %s would be added to the inbox. Nothing has been written.\n", plural(total, "note"))
	case backfill && total > 0:
		fmt.Printf("\n%s added to the inbox. The next `regesto cycle` turns them into facts; review inbox/ before then if you like.\n", plural(total, "note"))
	default:
		fmt.Printf("\n%s added to the inbox.\n", plural(total, "note"))
	}
}
