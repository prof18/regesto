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
)

// runCycle is the whole downstream pass in one command: normalise the inbox,
// reconcile, rebuild the generated artifacts, commit.
//
// It exists so a scheduler has a single thing to call. Harvest is deliberately
// not part of it — harvest runs on every machine, while this runs only where
// write authority lives (DECISION §9.0), and conflating them would put two
// machines in the business of deciding what becomes a fact.
func runCycle(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("cycle", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "report what the pass would do; change nothing")
	command := fs.String("command", "", "agent command for normalisation")
	noCommit := fs.Bool("no-commit", false, "skip the git commit")
	push := fs.Bool("push", false, "push after committing (needs an offsite remote, PLAN 0.7)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if host := strings.TrimSpace(cfg.Section("roles")["lint"]); host != "" && host != cfg.Machine {
		// Refuse rather than proceed: a second machine running this would
		// mint vocabulary against a different INDEX.md and split the store.
		return fmt.Errorf("this machine is %q but [roles].lint names %q — cycle runs only there", cfg.Machine, host)
	}

	chain := normalize.Commands(*command, cfg.Section("normalize")["commands"])

	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}

	// 1. Normalise whatever the machines have captured.
	outcomes, err := normalize.Run(cfg, all, normalize.Options{
		Commands:       chain,
		DryRun:         *dry,
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
			fmt.Printf("normalize %s\n", a)
		}
		if o.Note != "" {
			fmt.Printf("normalize %s — %s\n", o.Capture.Path, o.Note)
		}
		for _, w := range o.Written {
			fmt.Printf("normalize wrote %s\n", w)
			written++
		}
		for _, r := range o.Rejected {
			fmt.Printf("normalize rejected %s\n", r)
		}
		if o.Archived != "" {
			fmt.Printf("normalize archived %s\n", o.Archived)
		}
	}
	if len(outcomes) == 0 {
		fmt.Println("normalize nothing in the inbox")
	}

	// 2. Resolve sync conflicts before validating, so collisions the sync
	// client created are not reported as store errors.
	conflicts, err := facts.FindConflicts(cfg.KBRoot)
	if err != nil {
		return err
	}
	resolutions, err := lint.ResolveConflicts(cfg.KBRoot, conflicts, !*dry, time.Now().UTC())
	if err != nil {
		return err
	}
	for _, r := range resolutions {
		state := "resolved"
		if r.NeedsHuman {
			state = "NEEDS HUMAN"
		} else if *dry {
			state = "would resolve"
		}
		fmt.Printf("conflict %s %s — %s\n", state, r.ConflictPath, r.Message)
		if r.Archived != "" {
			fmt.Printf("conflict archived %s\n", r.Archived)
		}
	}

	// 3. Validate and reconcile everything, including facts written directly by
	// regesto-write, which never pass through the inbox.
	all, err = facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}
	scopeFixes, err := lint.CanonicaliseScopes(cfg.KBRoot, cfg, all, !*dry, time.Now().UTC())
	if err != nil {
		return err
	}
	for _, s := range scopeFixes {
		if s.Blocked != "" {
			fmt.Printf("scope BLOCKED %s — %s; %s\n", s.ID, s.Message, s.Blocked)
			continue
		}
		fmt.Printf("scope %s %s — %s\n", map[bool]string{true: "would move", false: "moved"}[*dry], s.ID, s.Message)
	}
	if len(scopeFixes) > 0 && !*dry {
		if all, err = facts.LoadAll(cfg.KBRoot); err != nil {
			return err
		}
	}

	report := lint.Run(all, time.Now().UTC())
	for _, f := range report.Findings {
		fmt.Printf("lint %-5s %s: %s\n", f.Severity, f.Path, f.Message)
	}
	for _, a := range report.Actions {
		fmt.Printf("lint %s %s — %s\n", map[bool]string{true: "would", false: "did"}[*dry], a.ID, a.Message)
	}
	for _, r := range report.Reviews {
		fmt.Printf("lint review %s\n", r)
	}
	for _, d := range report.NearDuplicates {
		fmt.Printf("lint vocab %s\n", d)
	}
	for _, d := range report.Due {
		fmt.Printf("lint due %s\n", d)
	}

	if *dry {
		fmt.Printf("dry run — %d error(s), %d action(s) pending\n", report.Errors(), len(report.Actions))
		return nil
	}
	if report.Errors() > 0 {
		return fmt.Errorf("%d validation error(s); nothing applied, nothing committed", report.Errors())
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
	fmt.Printf("rebuilt INDEX.md and knowledge/topics/ from %d fact(s)\n", len(all))

	if *noCommit {
		return nil
	}
	return commit(cfg, written, len(report.Actions), *push)
}

// commit records the pass. Nothing here is fatal: the markdown files are the
// knowledge and git is convenience (DECISION §9.1), so a failure to commit must
// not fail the cycle or the next one will not run either.
func commit(cfg *config.Config, written, actions int, push bool) error {
	if _, err := os.Stat(cfg.KBRoot + "/.git"); err != nil {
		fmt.Println("commit skipped — not a git repository")
		return nil
	}
	if out, err := gitRun(cfg.KBRoot, "git", "status", "--porcelain"); err != nil {
		fmt.Println("commit skipped —", err)
		return nil
	} else if strings.TrimSpace(out) == "" {
		fmt.Println("commit nothing to commit")
		return nil
	}
	if _, err := gitRun(cfg.KBRoot, "git", "add", "-A"); err != nil {
		fmt.Println("commit skipped —", err)
		return nil
	}
	msg := fmt.Sprintf("regesto cycle: %d fact(s) normalised, %d reconciliation action(s)", written, actions)
	if _, err := gitRun(cfg.KBRoot, "git", "commit", "-m", msg); err != nil {
		fmt.Println("commit failed —", err)
		return nil
	}
	fmt.Println("commit " + msg)

	if push {
		if _, err := gitRun(cfg.KBRoot, "git", "push"); err != nil {
			fmt.Println("push failed —", err)
			return nil
		}
		fmt.Println("push ok")
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
