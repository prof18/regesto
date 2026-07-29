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
	apply := fs.Bool("fix", false, "apply the mechanical changes; without this, report only")
	rebuild := fs.Bool("rebuild", false, "also regenerate INDEX.md and knowledge/topics/")
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
	for _, r := range resolutions {
		verb := "would resolve"
		if *apply {
			verb = "resolved    "
		}
		if r.NeedsHuman {
			verb = "NEEDS HUMAN "
		}
		fmt.Printf("conflict %s %s — %s\n", verb, r.ConflictPath, r.Message)
		if r.Archived != "" {
			fmt.Printf("conflict              archived %s\n", r.Archived)
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
	for _, s := range scopeFixes {
		if s.Blocked != "" {
			fmt.Printf("scope    BLOCKED  %s — %s; %s\n", s.ID, s.Message, s.Blocked)
			continue
		}
		verb := "would move"
		if *apply {
			verb = "moved    "
		}
		fmt.Printf("scope    %s %s — %s\n", verb, s.ID, s.Message)
	}
	if len(scopeFixes) > 0 && *apply {
		if all, err = facts.LoadAll(cfg.KBRoot); err != nil {
			return err
		}
	}

	report := lint.Run(all, time.Now().UTC())

	if !*quiet {
		fmt.Printf("checked %d fact(s)\n", report.Checked)
	}

	for _, f := range report.Findings {
		fmt.Printf("%-5s %s: %s\n", f.Severity, f.Path, f.Message)
	}

	// Report before applying: the run summary is the whole point, and a
	// wrongly-merged pair is only catchable if you see it happen.
	for _, a := range report.Actions {
		verb := "would"
		if *apply {
			verb = "did  "
		}
		fmt.Printf("%s %-10s %s — %s\n", verb, a.Kind, a.ID, a.Message)
	}
	for _, r := range report.Reviews {
		fmt.Printf("review    %s\n", r)
	}
	for _, d := range report.NearDuplicates {
		fmt.Printf("vocab     %s\n", d)
	}
	for _, d := range report.Due {
		fmt.Printf("due       %s\n", d)
	}

	if *apply {
		// Refuse to mutate a store that does not parse cleanly: reconciliation
		// decided what to do from data that validation just called wrong.
		if n := report.Errors(); n > 0 {
			return fmt.Errorf("%d error(s) — fix those before --fix, so changes are not computed from bad data", n)
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
		if !*quiet {
			fmt.Printf("rebuilt INDEX.md and knowledge/topics/\n")
		}
	}

	if !*quiet {
		fmt.Printf("%d error(s), %d warning(s), %d action(s), %d pending review(s)\n",
			report.Errors(), len(report.Findings)-report.Errors(), len(report.Actions), len(report.Reviews))
	}
	if report.Errors() > 0 {
		os.Exit(1)
	}
	return nil
}
