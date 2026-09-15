package main

import (
	"fmt"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/index"
)

func runIndex(cfg *config.Config) error {
	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return err
	}
	r := index.Build(all)
	if err := index.Write(cfg.KBRoot, r); err != nil {
		return err
	}
	fmt.Printf("Rebuilt INDEX.md and %s from %s.\n", plural(len(r.TopicPages), "topic page"), plural(len(all), "fact"))
	return nil
}
