package main

import (
	"flag"
	"fmt"
	"os"

	"regesto/internal/config"
	"regesto/internal/facts"
	"regesto/internal/index"
	"regesto/internal/project"
)

func runContext(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	dir := fs.String("dir", "", "working directory to resolve the project from (default: cwd)")
	projectName := fs.String("project", "", "override the canonical project name")
	maxBytes := fs.Int("max-bytes", 4096, "cap the payload; 0 for no cap")
	vocabulary := fs.Bool("vocabulary", false, "include the full controlled-vocabulary table")
	debug := fs.Bool("debug", false, "report how the project name was resolved, on stderr")
	if err := fs.Parse(args); err != nil {
		return err
	}

	from := *dir
	if from == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		from = cwd
	}

	name := *projectName
	if name == "" {
		r := project.Resolve(cfg, from)
		name = r.Name
		if *debug {
			fmt.Fprintf(os.Stderr, "project %q resolved via %s (mapped=%v)\n", r.Name, r.How, r.Mapped)
		}
	}

	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}
	fmt.Print(index.BuildContext(all, index.ContextOptions{
		Project:    name,
		MaxBytes:   *maxBytes,
		Vocabulary: *vocabulary,
	}))
	return nil
}
