package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/index"
	"github.com/prof18/regesto/internal/project"
)

func runContext(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	dir := fs.String("dir", "", "working directory to resolve the project from (default: cwd)")
	projectName := fs.String("project", "", "override the canonical project name")
	maxBytes := fs.Int("max-bytes", 4096, "cap the payload; 0 for no cap")
	vocabulary := fs.Bool("vocabulary", false, "include the full controlled-vocabulary table")
	debug := fs.Bool("debug", false, "report how the project name was resolved, on stderr")
	jsonOutput := fs.Bool("json", false, "print context and project resolution as JSON")
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

	r := project.Resolution{Name: *projectName, How: "flag"}
	if *projectName == "" {
		r = project.Resolve(cfg, from)
		if *debug {
			fmt.Fprintf(os.Stderr, "project %q resolved via %s (mapped=%v)\n", r.Name, r.How, r.Mapped)
		}
	}

	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}
	context := index.BuildContext(all, index.ContextOptions{
		Project:    r.Name,
		MaxBytes:   *maxBytes,
		Vocabulary: *vocabulary,
	})
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(jsonContextResponse{
			SchemaVersion: jsonSchemaVersion,
			Context:       context,
			Project:       jsonContextProject{Name: r.Name, Resolution: jsonProjectResolution(r)},
		})
	}
	fmt.Print(context)
	return nil
}
