package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/index"
	"github.com/prof18/regesto/internal/notify"
	"github.com/prof18/regesto/internal/project"
)

func runContext(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	setUsage(fs, "regesto context [options]", "Print the knowledge base summary an agent receives at the start of a session.")
	dir := fs.String("dir", "", "project folder to build it for (default: current folder)")
	projectName := fs.String("project", "", "use this project name instead of detecting it")
	maxBytes := fs.Int("max-bytes", 4096, "maximum size in bytes; 0 for no limit")
	vocabulary := fs.Bool("vocabulary", false, "include the full list of subjects and relations in use")
	debug := fs.Bool("debug", false, "print where the project name came from (to stderr)")
	jsonOutput := fs.Bool("json", false, "print the result as JSON")
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
			fmt.Fprintf(os.Stderr, "project %q (%s)\n", r.Name, projectOrigin(r))
		}
	}

	alert := notify.ContextAlert(cfg, "cycle", time.Now().UTC())
	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		// Preserve the CLI's non-zero failure contract, but do not hide the
		// already-recorded maintenance warning from a human invoking it.
		if context := index.BuildMaintenanceContext(alert, *maxBytes); context != "" {
			fmt.Fprint(os.Stderr, context)
		}
		return err
	}
	context := index.BuildContext(all, index.ContextOptions{
		Project:    r.Name,
		Alert:      alert,
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
