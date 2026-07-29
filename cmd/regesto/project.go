package main

import (
	"flag"
	"fmt"
	"os"

	"regesto/internal/config"
	"regesto/internal/project"
)

// runProject prints the canonical project name for a directory. The write path
// needs this to choose a fact's `scope:`, and asking an agent to re-derive it
// from the git remote plus the [projects] map by hand invites a different answer
// from the one the SessionStart hook used for the same repo.
func runProject(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("project", flag.ContinueOnError)
	dir := fs.String("dir", "", "directory to resolve from (default: cwd)")
	scope := fs.Bool("scope", false, "print as a scope value, i.e. project:<name>")
	verbose := fs.Bool("v", false, "also print how the name was resolved")
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
	name := r.Name
	if *scope {
		name = "project:" + name
	}
	if *verbose {
		fmt.Printf("%s\t(via %s, mapped=%v)\n", name, r.How, r.Mapped)
		return nil
	}
	fmt.Println(name)
	return nil
}
