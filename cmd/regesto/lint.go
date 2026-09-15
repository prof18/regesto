package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/index"
	"github.com/prof18/regesto/internal/lint"
)

func runLint(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	setUsage(fs, "regesto lint [options]", "Check every fact for problems, and find facts that contradict or replace each other.")
	apply := fs.Bool("fix", false, "apply the changes it suggests (without this, it only reports)")
	rebuild := fs.Bool("rebuild", false, "also rebuild INDEX.md and the topic pages")
	quiet := fs.Bool("quiet", false, "print only problems and changes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Conflicts first: an unresolved copy holds a duplicate id, so resolving
	// them before validating avoids reporting collisions the sync client
	// created rather than the store.
	conflicts, err := facts.FindConflicts(cfg.KBRoot)
	if err != nil {
		return err
	}
	resolutions, err := lint.ResolveConflicts(cfg.KBRoot, conflicts, *apply, time.Now().UTC())
	if err != nil {
		return err
	}
	if len(resolutions) > 0 {
		fmt.Println("Sync conflicts")
	}
	for _, r := range resolutions {
		verb := "Would resolve"
		if *apply {
			verb = "Resolved"
		}
		if r.NeedsHuman {
			verb = "NEEDS YOU"
		}
		fmt.Printf("  %-13s %s: %s\n", verb, r.ConflictPath, r.Message)
		if r.Archived != "" {
			fmt.Printf("  %-13s %s\n", "Archived", r.Archived)
		}
	}

	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}

	// Project-name drift before validation: a fact filed under an alias splits
	// a project's knowledge in half, so a scoped session sees only part of it.
	scopeFixes, err := lint.CanonicaliseScopes(cfg.KBRoot, cfg, all, *apply, time.Now().UTC())
	if err != nil {
		return err
	}
	if len(scopeFixes) > 0 {
		fmt.Println("Project names")
	}
	for _, s := range scopeFixes {
		if s.Blocked != "" {
			fmt.Printf("  %-13s %s: %s; %s\n", "BLOCKED", s.ID, s.Message, s.Blocked)
			continue
		}
		verb := "Would move"
		if *apply {
			verb = "Moved"
		}
		fmt.Printf("  %-13s %s: %s\n", verb, s.ID, s.Message)
	}
	if len(scopeFixes) > 0 && *apply {
		if all, err = facts.LoadAll(cfg.KBRoot); err != nil {
			return err
		}
	}

	report := lint.Run(all, time.Now().UTC())

	if len(report.Findings) > 0 {
		fmt.Println("\nProblems")
	}
	for _, f := range report.Findings {
		fmt.Printf("  %-7s %s: %s\n", severityLabel(f.Severity), f.Path, f.Message)
	}

	// Report before applying: the run summary is the whole point, and a
	// wrongly-merged pair is only catchable if you see it happen.
	if len(report.Actions) > 0 {
		if *apply {
			fmt.Println("\nChanges made")
		} else {
			fmt.Println("\nChanges to make (run with --fix to apply)")
		}
	}
	for _, a := range report.Actions {
		fmt.Printf("  %s: %s\n", a.ID, a.Message)
	}
	if len(report.Reviews) > 0 {
		fmt.Println("\nWaiting for your review")
	}
	for _, r := range report.Reviews {
		fmt.Printf("  %s\n", r)
	}
	if len(report.NearDuplicates) > 0 {
		fmt.Println("\nPossible duplicate names")
	}
	for _, d := range report.NearDuplicates {
		fmt.Printf("  %s\n", d)
	}
	if len(report.Due) > 0 {
		fmt.Println("\nDue for review")
	}
	for _, d := range report.Due {
		fmt.Printf("  %s\n", d)
	}

	if *apply {
		// Refuse to mutate a store that does not parse cleanly: reconciliation
		// decided what to do from data that validation just called wrong.
		if n := report.Errors(); n > 0 {
			return fmt.Errorf("%s found; fix them first, then run --fix again (nothing was changed)", plural(n, "error"))
		}
		for _, a := range report.Actions {
			path := cfg.KBRoot + "/" + a.Path
			if err := facts.SetFields(path, a.Updates); err != nil {
				return fmt.Errorf("applying %s to %s: %w", a.Kind, a.ID, err)
			}
		}
	}

	if *rebuild {
		all, err = facts.LoadAll(cfg.KBRoot)
		if err != nil {
			return err
		}
		if err := index.Write(cfg.KBRoot, index.Build(all)); err != nil {
			return err
		}
	}

	if !*quiet {
		errors, warnings := report.Errors(), len(report.Findings)-report.Errors()
		if len(report.Findings)+len(report.Actions)+len(report.Reviews)+len(report.NearDuplicates)+len(report.Due)+len(resolutions)+len(scopeFixes) > 0 {
			fmt.Println()
		}
		summary := fmt.Sprintf("Checked %s: %s, %s", plural(report.Checked, "fact"), plural(errors, "error"), plural(warnings, "warning"))
		if n := len(report.Actions); n > 0 {
			if *apply {
				summary += ", " + plural(n, "change") + " made"
			} else {
				summary += ", " + plural(n, "change") + " to make"
			}
		}
		if n := len(report.Reviews); n > 0 {
			summary += ", " + plural(n, "fact") + " waiting for review"
		}
		fmt.Println(summary + ".")
		if *rebuild {
			fmt.Println("Rebuilt INDEX.md and the topic pages.")
		}
	}
	if report.Errors() > 0 {
		os.Exit(1)
	}
	return nil
}

func severityLabel(s lint.Severity) string {
	if s == lint.Error {
		return "Error"
	}
	return "Warning"
}
