package main

import (
	"flag"
	"fmt"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/harvest"
)

func runHarvest(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("harvest", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "report what would be captured; write nothing")
	verbose := fs.Bool("v", false, "list skipped files too")
	if err := fs.Parse(args); err != nil {
		return err
	}

	results, err := harvest.Run(cfg, *dry)
	if err != nil {
		return err
	}
	total := 0
	for _, r := range results {
		label := r.Agent
		if r.SourceID != "" {
			label += "[" + r.SourceID + "]"
		}
		if r.Note != "" {
			fmt.Printf("%s %s\n", label, r.Note)
		}
		for _, c := range r.Captured {
			fmt.Printf("%s captured %s\n", label, c)
		}
		if *verbose {
			for _, s := range r.Skipped {
				fmt.Printf("%s skipped  %s\n", label, s)
			}
		}
		if r.Note == "" && len(r.Captured) == 0 {
			fmt.Printf("%s scanned %d, nothing new\n", label, r.Scanned)
		}
		total += len(r.Captured)
	}
	if *dry {
		fmt.Printf("dry run — %d capture(s) would be written to inbox/\n", total)
	} else {
		fmt.Printf("%d capture(s) written to inbox/\n", total)
	}
	return nil
}
