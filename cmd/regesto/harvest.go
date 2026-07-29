package main

import (
	"flag"
	"fmt"

	"regesto/internal/config"
	"regesto/internal/harvest"
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
		if r.Note != "" {
			fmt.Printf("%-8s %s\n", r.Agent, r.Note)
		}
		for _, c := range r.Captured {
			fmt.Printf("%-8s captured %s\n", r.Agent, c)
		}
		if *verbose {
			for _, s := range r.Skipped {
				fmt.Printf("%-8s skipped  %s\n", r.Agent, s)
			}
		}
		if r.Note == "" && len(r.Captured) == 0 {
			fmt.Printf("%-8s scanned %d, nothing new\n", r.Agent, r.Scanned)
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
