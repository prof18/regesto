package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"regesto/internal/config"
	"regesto/internal/facts"
	"regesto/internal/lint"
	"regesto/internal/normalize"
)

// runPromote turns a chat transcript into facts (PLAN 2.e).
//
// Input is text on stdin or a file, deliberately. The realistic ways to get a
// conversation out of a chat app without publishing a public link are the
// account-wide data export and copy-paste, and both end as text. Reading stdin
// also means a future Hermes channel can pipe a message straight in without this
// command learning anything about Hermes.
func runPromote(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("promote", flag.ContinueOnError)
	source := fs.String("source", "human", "value for the facts' `source:` field")
	name := fs.String("name", "", "label for the archived transcript (default: the filename, or `pasted`)")
	dry := fs.Bool("dry-run", false, "report the prompt size and stop; write nothing")
	command := fs.String("command", "", "single agent command, overriding the fallback chain")
	maxPrompt := fs.Int("max-prompt-bytes", -1, "prompt ceiling; 0 for no limit (default 65536)")
	timeout := fs.Duration("timeout", 10*time.Minute, "time limit for the agent")
	// Go's flag package stops at the first positional argument, so
	// `promote file.md --name x` would silently ignore --name. Reading a file
	// name first is the natural way to type this, so accept either order.
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}

	var (
		transcript []byte
		err        error
		label      = *name
	)
	switch {
	case fs.NArg() == 0 || fs.Arg(0) == "-":
		stat, _ := os.Stdin.Stat()
		if stat != nil && stat.Mode()&os.ModeCharDevice != 0 {
			return fmt.Errorf("no input — pass a file, or pipe the transcript in:\n" +
				"  regesto promote conversation.md\n" +
				"  pbpaste | regesto promote -")
		}
		transcript, err = io.ReadAll(os.Stdin)
		if label == "" {
			label = "pasted"
		}
	default:
		path := fs.Arg(0)
		transcript, err = os.ReadFile(path)
		if label == "" {
			label = filepath.Base(path)
		}
	}
	if err != nil {
		return err
	}

	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}

	res, err := normalize.Promote(cfg, all, string(transcript), normalize.PromoteOptions{
		Options: normalize.Options{
			Commands:       normalize.Commands(*command, cfg.Section("normalize")["commands"]),
			DryRun:         *dry,
			Timeout:        *timeout,
			MaxPromptBytes: limitFlag(*maxPrompt),
		},
		Projects: lint.KnownProjects(cfg, all),
		Source:   *source,
		Name:     label,
	})
	if err != nil {
		return err
	}

	for _, a := range res.Attempts {
		fmt.Printf("  %s\n", a)
	}
	if res.UsedCommand != "" {
		fmt.Printf("  via      %s\n", res.UsedCommand)
	}
	if res.Note != "" {
		fmt.Printf("  %s\n", res.Note)
	}
	for _, w := range res.Written {
		fmt.Printf("  wrote    %s\n", w)
	}
	for _, r := range res.Rejected {
		fmt.Printf("  rejected %s\n", r)
	}
	if res.Archived != "" {
		fmt.Printf("  archived %s\n", res.Archived)
	}
	if len(res.Written) > 0 {
		fmt.Println("\nreview what was extracted, then `regesto lint --fix --rebuild`")
	}
	return nil
}

// flagsFirst moves positional arguments after the flags, so both
// `promote --name x file.md` and `promote file.md --name x` work. A bare `-`
// means stdin and is positional; anything else starting with `-` is a flag, and
// a flag that takes a value keeps the value with it.
func flagsFirst(args []string) []string {
	valueFlags := map[string]bool{
		"--source": true, "-source": true,
		"--name": true, "-name": true,
		"--command": true, "-command": true,
		"--max-prompt-bytes": true, "-max-prompt-bytes": true,
		"--timeout": true, "-timeout": true,
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-" || !strings.HasPrefix(a, "-"):
			positional = append(positional, a)
		case valueFlags[a] && i+1 < len(args):
			flags = append(flags, a, args[i+1])
			i++
		default:
			flags = append(flags, a)
		}
	}
	return append(flags, positional...)
}
