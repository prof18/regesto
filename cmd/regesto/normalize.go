package main

import (
	"flag"
	"fmt"
	"time"

	"regesto/internal/config"
	"regesto/internal/facts"
	"regesto/internal/lint"
	"regesto/internal/normalize"
)

func runNormalize(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("normalize", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "list captures and prompt sizes; invoke nothing, write nothing")
	command := fs.String("command", "", "single agent command, overriding the fallback chain")
	timeout := fs.Duration("timeout", 5*time.Minute, "per-capture time limit")
	showPrompt := fs.Bool("show-prompt", false, "print the prompt for the first capture and stop")
	maxPrompt := fs.Int("max-prompt-bytes", -1, "defer captures whose prompt exceeds this; 0 for no limit (default 65536)")
	maxCaptures := fs.Int("max-captures", -1, "process at most this many captures per run; 0 for no limit (default 20)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}

	if *showPrompt {
		captures, err := normalize.Find(cfg.KBRoot)
		if err != nil {
			return err
		}
		if len(captures) == 0 {
			fmt.Println("no captures in inbox/")
			return nil
		}
		var vocab, ids []string
		for _, f := range all {
			vocab = append(vocab, fmt.Sprintf("(%s, %s)", f.Subject, f.Relation))
			ids = append(ids, f.ID)
		}
		fmt.Print(normalize.Prompt(captures[0], vocab, ids, lint.KnownProjects(cfg, all)...))
		return nil
	}

	chain := normalize.Commands(*command, cfg.Section("normalize")["commands"])

	outcomes, err := normalize.Run(cfg, all, normalize.Options{
		Commands:       chain,
		DryRun:         *dry,
		Timeout:        *timeout,
		Projects:       lint.KnownProjects(cfg, all),
		MaxPromptBytes: limitFlag(*maxPrompt),
		MaxCaptures:    limitFlag(*maxCaptures),
	})
	if err != nil {
		return err
	}
	if len(outcomes) == 0 {
		fmt.Println("no captures in inbox/")
		return nil
	}

	written, rejected := 0, 0
	for _, o := range outcomes {
		fmt.Printf("%s\n", o.Capture.Path)
		for _, a := range o.Attempts {
			fmt.Printf("    %s\n", a)
		}
		if o.UsedCommand != "" {
			fmt.Printf("    via      %s\n", o.UsedCommand)
		}
		if o.Note != "" {
			fmt.Printf("    %s\n", o.Note)
		}
		for _, w := range o.Written {
			fmt.Printf("    wrote    %s\n", w)
			written++
		}
		// Rejections are printed, never swallowed: a candidate that failed a
		// check is the most interesting thing in the run.
		for _, r := range o.Rejected {
			fmt.Printf("    rejected %s\n", r)
			rejected++
		}
	}
	fmt.Printf("%d fact(s) written, %d candidate(s) rejected\n", written, rejected)
	if written > 0 {
		fmt.Println("run `regesto lint --fix --rebuild` to reconcile and regenerate")
	}
	return nil
}

// limitFlag translates the CLI convention into the internal one. On the command
// line 0 means "no limit", which is what the deferral message tells you to pass
// and what people expect; internally 0 is the zero value and has to mean "unset"
// so an omitted field is safe rather than unlimited.
func limitFlag(v int) int {
	switch {
	case v < 0:
		return 0 // flag absent: unset, use the default
	case v == 0:
		return -1 // explicitly no limit
	default:
		return v
	}
}
