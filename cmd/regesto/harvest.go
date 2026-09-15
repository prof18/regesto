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
	setUsage(fs, "regesto harvest [options]", "Collect new or changed agent memory files into the inbox.")
	dry := fs.Bool("dry-run", false, "show what would be collected without writing anything")
	verbose := fs.Bool("v", false, "also list the files that were skipped, and why")
	if err := fs.Parse(args); err != nil {
		return err
	}

	results, err := harvest.Run(cfg, *dry)
	if err != nil {
		return err
	}
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
		if *verbose {
			for _, s := range r.Skipped {
				fmt.Printf("%s: skipped %s\n", label, s)
			}
		}
		if r.Note == "" && len(r.Captured) == 0 {
			fmt.Printf("%s: nothing new (scanned %s)\n", label, plural(r.Scanned, "file"))
		}
		total += len(r.Captured)
	}
	if *dry {
		fmt.Printf("\nDry run: %s would be added to the inbox. Nothing has been written.\n", plural(total, "note"))
	} else {
		fmt.Printf("\n%s added to the inbox.\n", plural(total, "note"))
	}
	return nil
}
