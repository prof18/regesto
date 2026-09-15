package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/project"
)

// runProject prints the canonical project name for a directory. The write path
// needs this to choose a fact's `scope:`, and asking an agent to re-derive it
// from the git remote plus the [projects] map by hand invites a different answer
// from the one the SessionStart hook used for the same repo.
// projectOrigin says in words where a project name came from.
func projectOrigin(r project.Resolution) string {
	origin := map[string]string{
		"config-path":  "set for this folder in [projects] in config.toml",
		"git-remote":   "from the git remote",
		"dir-basename": "from the folder name",
		"flag":         "given with --project",
	}[r.How]
	if origin == "" {
		origin = r.How
	}
	if r.Mapped && r.How != "config-path" {
		origin += ", renamed by [projects] in config.toml"
	}
	return origin
}

func runProject(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("project", flag.ContinueOnError)
	setUsage(fs, "regesto project [options]", "Print the project name Regesto uses for a folder.")
	dir := fs.String("dir", "", "folder to check (default: current folder)")
	scope := fs.Bool("scope", false, "print it as a scope, i.e. project:<name>")
	verbose := fs.Bool("v", false, "also say where the name came from")
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

	r := project.Resolve(cfg, from)
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(jsonProjectResponse{
			SchemaVersion: jsonSchemaVersion,
			Project:       r.Name,
			Scope:         "project:" + r.Name,
			Resolution:    jsonProjectResolution(r),
		})
	}
	name := r.Name
	if *scope {
		name = "project:" + name
	}
	if *verbose {
		fmt.Printf("%s\t(%s)\n", name, projectOrigin(r))
		return nil
	}
	fmt.Println(name)
	return nil
}
