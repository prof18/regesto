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
	fmt.Printf("indexed %d fact(s): INDEX.md + %d topic page(s) rebuilt\n", len(all), len(r.TopicPages))
	return nil
}
