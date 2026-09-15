package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/harvest"
)

// offerMemoryImport asks, once, whether the memory the chosen agents already
// have should be copied into the inbox. Without it the first harvest only
// records that memory as a starting point and never collects it.
//
// It only fills the inbox; turning notes into facts is left to `regesto cycle`
// so the import can be reviewed first. It returns how many notes were added.
// A failure here never fails init: the knowledge base is already usable, and
// `regesto harvest --backfill` does the same thing later.
func offerMemoryImport(in io.Reader, out io.Writer, configPath string) int {
	later := "Run `regesto harvest --backfill` any time to import it."
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(out, "Could not check your agents' existing memory: %v\n%s\n\n", err, later)
		return 0
	}
	preview, err := harvest.RunWithOptions(cfg, harvest.Options{DryRun: true, Backfill: true})
	if err != nil {
		fmt.Fprintf(out, "Could not check your agents' existing memory: %v\n%s\n\n", err, later)
		return 0
	}
	counts, order := map[string]int{}, []string{}
	total := 0
	for _, r := range preview {
		if len(r.Captured) == 0 {
			continue
		}
		if _, seen := counts[r.Agent]; !seen {
			order = append(order, r.Agent)
		}
		counts[r.Agent] += len(r.Captured)
		total += len(r.Captured)
	}
	if total == 0 {
		return 0
	}

	fmt.Fprintln(out, "Your agents already have memory from before Regesto:")
	for _, id := range order {
		fmt.Fprintf(out, "  %-14s %s\n", adapters.DisplayName(id), plural(counts[id], "file"))
	}
	fmt.Fprintln(out, "\nImport it into the inbox? The next `regesto cycle` turns it into facts,")
	fmt.Fprint(out, "so you can look through inbox/ first. [y/N] ")
	answer, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
	default:
		fmt.Fprintf(out, "Skipped. %s\n\n", later)
		return 0
	}

	results, err := harvest.RunWithOptions(cfg, harvest.Options{Backfill: true})
	imported := 0
	for _, r := range results {
		imported += len(r.Captured)
	}
	if err != nil {
		fmt.Fprintf(out, "Import stopped after %s: %v\n%s\n\n", plural(imported, "note"), err, later)
		return imported
	}
	fmt.Fprintf(out, "Imported %s into the inbox.\n\n", plural(imported, "note"))
	return imported
}
