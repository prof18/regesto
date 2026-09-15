package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/index"
	"github.com/prof18/regesto/internal/lint"
	"github.com/prof18/regesto/internal/normalize"
	"github.com/prof18/regesto/internal/notify"
)

// runCycle is the whole downstream pass in one command: normalise the inbox,
// reconcile, rebuild the generated artifacts, commit.
//
// It exists so a scheduler has a single thing to call. Harvest is deliberately
// not part of it — harvest runs on every machine, while this runs only where
// write authority lives (DESIGN §9.0), and conflating them would put two
// machines in the business of deciding what becomes a fact.
func runCycle(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("cycle", flag.ContinueOnError)
	setUsage(fs, "regesto cycle [options]", "Do all the upkeep in one go: turn inbox notes into facts, fix up facts,\nrebuild the index, and commit to git. This is what the hourly job runs.")
	dry := fs.Bool("dry-run", false, "show what would happen without changing anything")
	command := fs.String("command", "", "agent `command` to use, instead of [normalize] commands in config.toml")
	noCommit := fs.Bool("no-commit", false, "don't commit to git")
	push := fs.Bool("push", false, "push to the git remote after committing")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if host := strings.TrimSpace(cfg.Section("roles")["lint"]); host != "" && host != cfg.Machine {
		// Refuse rather than proceed: a second machine running this would
		// mint vocabulary against a different INDEX.md and split the store.
		//
		// Deliberately before the pass and its health report: this machine was
		// never meant to run the cycle, so its refusal is not a failure of the
		// knowledge base and must not alert anyone.
		return fmt.Errorf("cycle only runs on %q, as set by [roles] lint in config.toml; this machine is %q", host, cfg.Machine)
	}

	err := cyclePass(cfg, cycleOptions{
		DryRun:   *dry,
		Command:  *command,
		NoCommit: *noCommit,
		Push:     *push,
	})
	// A dry run is someone looking, and someone looking needs no telling.
	if !*dry {
		reportCycleHealth(cfg, err, time.Now().UTC())
	}
	return err
}

type cycleOptions struct {
	DryRun   bool
	Command  string // agent invocation for normalisation; empty means configured
	NoCommit bool
	Push     bool
}

// reportCycleHealth is what makes a silent scheduled failure audible. It is
// never allowed to change the outcome of the pass: a notifier that is missing,
// misconfigured or broken is a worse reason to fail than the one it was trying
// to report.
func reportCycleHealth(cfg *config.Config, passErr error, now time.Time) {
	h := notify.Health{Key: "cycle", Failing: passErr != nil}
	if passErr != nil {
		h.Title = "Regesto: processing is failing"
		h.Message = passErr.Error() + "\nNew notes wait in the inbox until this is fixed."
	} else {
		h.Title = "Regesto: processing works again"
		h.Message = "All facts pass their checks, and the knowledge base is up to date."
	}
	if _, err := notify.Report(cfg, h, now); err != nil {
		fmt.Fprintln(os.Stderr, "Couldn't send the alert:", err)
	}
}

func cyclePass(cfg *config.Config, opts cycleOptions) error {
	chain := normalize.CommandsFor(opts.Command, cfg.Section("normalize")["commands"], cfg.IntegrationIDs())

	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}

	// 1. Normalise whatever the machines have captured.
	outcomes, err := normalize.Run(cfg, all, normalize.Options{
		Commands:       chain,
		DryRun:         opts.DryRun,
		Timeout:        5 * time.Minute,
		Projects:       lint.KnownProjects(cfg, all),
		MaxPromptBytes: intSetting(cfg, "max_prompt_bytes"),
		MaxCaptures:    intSetting(cfg, "max_captures"),
	})
	if err != nil {
		return err
	}
	written := 0
	for _, o := range outcomes {
		for _, a := range o.Attempts {
			fmt.Printf("inbox    %s\n", a)
		}
		if o.Note != "" {
			fmt.Printf("inbox    %s: %s\n", o.Capture.Path, o.Note)
		}
		for _, w := range o.Written {
			fmt.Printf("inbox    wrote %s\n", w)
			written++
		}
		for _, r := range o.Rejected {
			fmt.Printf("inbox    rejected %s\n", r)
		}
		if o.Archived != "" {
			fmt.Printf("inbox    done with %s\n", o.Archived)
		}
	}
	if len(outcomes) == 0 {
		fmt.Println("inbox    empty")
	}

	// 2. Resolve sync conflicts before validating, so collisions the sync
	// client created are not reported as store errors.
	conflicts, err := facts.FindConflicts(cfg.KBRoot)
	if err != nil {
		return err
	}
	resolutions, err := lint.ResolveConflicts(cfg.KBRoot, conflicts, !opts.DryRun, time.Now().UTC())
	if err != nil {
		return err
	}
	for _, r := range resolutions {
		state := "resolved"
		if r.NeedsHuman {
			state = "NEEDS YOU"
		} else if opts.DryRun {
			state = "would resolve"
		}
		fmt.Printf("sync     %s %s: %s\n", state, r.ConflictPath, r.Message)
		if r.Archived != "" {
			fmt.Printf("sync     archived %s\n", r.Archived)
		}
	}

	// 3. Validate and reconcile everything, including facts written directly by
	// regesto-write, which never pass through the inbox.
	all, err = facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}
	scopeFixes, err := lint.CanonicaliseScopes(cfg.KBRoot, cfg, all, !opts.DryRun, time.Now().UTC())
	if err != nil {
		return err
	}
	for _, s := range scopeFixes {
		if s.Blocked != "" {
			fmt.Printf("project  BLOCKED %s: %s; %s\n", s.ID, s.Message, s.Blocked)
			continue
		}
		fmt.Printf("project  %s %s: %s\n", map[bool]string{true: "would move", false: "moved"}[opts.DryRun], s.ID, s.Message)
	}
	if len(scopeFixes) > 0 && !opts.DryRun {
		if all, err = facts.LoadAll(cfg.KBRoot); err != nil {
			return err
		}
	}

	report := lint.Run(all, time.Now().UTC())
	for _, f := range report.Findings {
		fmt.Printf("check    %s %s: %s\n", strings.ToLower(severityLabel(f.Severity)), f.Path, f.Message)
	}
	for _, a := range report.Actions {
		fmt.Printf("check    %s%s: %s\n", map[bool]string{true: "would ", false: ""}[opts.DryRun], a.ID, a.Message)
	}
	for _, r := range report.Reviews {
		fmt.Printf("review   %s\n", r)
	}
	for _, d := range report.NearDuplicates {
		fmt.Printf("names    %s\n", d)
	}
	for _, d := range report.Due {
		fmt.Printf("due      %s\n", d)
	}

	if opts.DryRun {
		fmt.Printf("\nDry run: %s, %s to make. Nothing has been changed.\n", plural(report.Errors(), "error"), plural(len(report.Actions), "change"))
		return nil
	}
	if report.Errors() > 0 {
		return fmt.Errorf("%s in facts, so nothing was changed or committed%s",
			plural(report.Errors(), "error"), firstError(report))
	}
	for _, a := range report.Actions {
		if err := facts.SetFields(cfg.KBRoot+"/"+a.Path, a.Updates); err != nil {
			return err
		}
	}

	// 4. Rebuild the generated artifacts from the reconciled store.
	all, err = facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}
	if err := index.Write(cfg.KBRoot, index.Build(all)); err != nil {
		return err
	}
	fmt.Printf("index    rebuilt from %s\n", plural(len(all), "fact"))

	if opts.NoCommit {
		return nil
	}
	return commit(cfg, written, len(report.Actions), opts.Push)
}

// firstError names one offending file in the failure message. A count on its
// own cannot be acted on from a notification or a log line, and the premise of
// both is that nobody is going to open the full report.
func firstError(report *lint.Report) string {
	for _, f := range report.Findings {
		if f.Severity == lint.Error {
			return fmt.Sprintf(" (first: %s: %s)", f.Path, f.Message)
		}
	}
	return ""
}

// commit records the pass. Nothing here is fatal: the markdown files are the
// knowledge and git is convenience (DESIGN §9.1), so a failure to commit must
// not fail the cycle or the next one will not run either.
func commit(cfg *config.Config, written, actions int, push bool) error {
	if _, err := os.Stat(cfg.KBRoot + "/.git"); err != nil {
		fmt.Println("git      not a git repository, not committing")
		return nil
	}
	if out, err := gitRun(cfg.KBRoot, "git", "status", "--porcelain"); err != nil {
		fmt.Println("git      not committing:", err)
		return nil
	} else if strings.TrimSpace(out) == "" {
		fmt.Println("git      nothing to commit")
		return nil
	}
	if _, err := gitRun(cfg.KBRoot, "git", "add", "-A"); err != nil {
		fmt.Println("git      not committing:", err)
		return nil
	}
	msg := fmt.Sprintf("regesto cycle: %s from the inbox, %s", plural(written, "new fact"), plural(actions, "fact update"))
	if _, err := gitRun(cfg.KBRoot, "git", "commit", "-m", msg); err != nil {
		fmt.Println("git      commit failed:", err)
		return nil
	}
	fmt.Println("git      committed: " + msg)

	if push {
		if _, err := gitRun(cfg.KBRoot, "git", "push"); err != nil {
			fmt.Println("git      push failed:", err)
			return nil
		}
		fmt.Println("git      pushed")
	}
	return nil
}

// intSetting reads a [normalize] integer. Unset means the default applies; a
// configured 0 means no limit, matching the command line.
func intSetting(cfg *config.Config, key string) int {
	raw := strings.TrimSpace(cfg.Section("normalize")[key])
	if raw == "" {
		return 0 // unset: the package default applies
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return limitFlag(n)
}

func gitRun(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
