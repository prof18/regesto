package index

import (
	"fmt"
	"strings"

	"github.com/prof18/regesto/internal/facts"
)

// ContextOptions controls the SessionStart payload (PLAN 1.c).
type ContextOptions struct {
	// Project is the canonical current project name; "" injects global only.
	Project string
	// MaxBytes caps the payload. This lands in every session, and
	// accumulation is unbounded (DESIGN §12), so the cap is the mechanism
	// that keeps a growing store from silently inflating every prompt.
	// Zero means no cap.
	MaxBytes int
	// Vocabulary includes the full controlled-vocabulary table. Off by
	// default: the write path reads it from INDEX.md on demand via the
	// regesto-write skill, so paying for it in every session is waste.
	Vocabulary bool
}

// BuildContext renders the compact payload a SessionStart hook injects: what
// exists in the KB and how to search it — not the knowledge itself. Fact bodies
// stay on disk behind bin/regesto-search.
//
// Facts are generated from facts/ directly rather than by slicing INDEX.md, so
// the payload can't go stale if the index hasn't been rebuilt.
func BuildContext(all []facts.Fact, opts ContextOptions) string {
	live := make([]facts.Fact, 0, len(all))
	for _, f := range all {
		if f.Status != facts.StatusSuperseded {
			live = append(live, f)
		}
	}

	globals := scopeFacts(live, "")
	var project []facts.Fact
	if opts.Project != "" {
		project = scopeFacts(live, opts.Project)
	}

	header := contextHeader(all)
	hasProject := opts.Project != "" && len(project) > 0
	globalBlock, _ := factBlock("Global", globals, -1)
	projectBlock := ""
	if hasProject {
		projectBlock, _ = factBlock("Project: "+opts.Project, project, -1)
	}
	vocab := ""
	if opts.Vocabulary {
		vocab = vocabularyBlock(all)
	}

	if opts.MaxBytes <= 0 {
		return header + globalBlock + projectBlock + vocab
	}
	// A payload that cannot retain the complete search header is worse than no
	// payload. Returning empty is also the only honest way to keep MaxBytes a
	// strict cap for impractically small values.
	if len(header) > opts.MaxBytes {
		return ""
	}
	base := header + globalBlock + projectBlock
	if len(base) <= opts.MaxBytes {
		if len(base)+len(vocab) <= opts.MaxBytes {
			return base + vocab
		}
		return base
	}

	// The displayed order stays global then project, but selection spends the
	// bounded line budget on current-project facts first. Reserve the exact
	// truncation footer throughout so the returned payload never exceeds cap.
	globalHead := factHeading("Global", globals)
	projectHead := ""
	if hasProject {
		projectHead = factHeading("Project: "+opts.Project, project)
	}
	dropped := len(globals) + len(project)
	fixed := len(header) + len(globalHead) + len(projectHead) + len(droppedFooter(dropped))
	if fixed > opts.MaxBytes {
		return ""
	}
	remaining := opts.MaxBytes - fixed
	projectLines, globalLines := make([]string, 0, len(project)), make([]string, 0, len(globals))
	selectLines := func(list []facts.Fact, selected *[]string) {
		for _, line := range factLines(list) {
			nextDropped := dropped - 1
			cost := len(line) + len(droppedFooter(nextDropped)) - len(droppedFooter(dropped))
			if cost > remaining {
				continue
			}
			*selected = append(*selected, line)
			remaining -= cost
			dropped = nextDropped
		}
	}
	selectLines(project, &projectLines)
	selectLines(globals, &globalLines)

	var b strings.Builder
	b.WriteString(header)
	b.WriteString(globalHead)
	for _, line := range globalLines {
		b.WriteString(line)
	}
	if hasProject {
		b.WriteString(projectHead)
		for _, line := range projectLines {
			b.WriteString(line)
		}
	}
	b.WriteString(droppedFooter(dropped))
	return b.String()
}

func contextHeader(all []facts.Fact) string {
	var b strings.Builder
	b.WriteString("# Knowledge base\n\n")
	b.WriteString("Canonical knowledge for this machine, technical and not. Consult it before\n")
	b.WriteString("any answer or decision a recorded claim could settle.\n\n")
	b.WriteString("- Search: `bin/regesto-search [--subject S] [--relation R] [--scope SC] [terms...]`\n")
	b.WriteString("- Read a claim in full at the path listed by that search.\n")
	b.WriteString("- Full manifest `INDEX.md` · contract `SCHEMA.md`\n")
	fmt.Fprintf(&b, "- %d `(subject, relation)` pairs in use — check `INDEX.md` before minting a new one.\n", len(vocabulary(all)))
	b.WriteString("- `[proposed]` claims await human review: rely on them only with a caveat.\n")
	b.WriteString("  Superseded claims are hidden unless you pass `--history`.\n")
	return b.String()
}

func vocabularyBlock(all []facts.Fact) string {
	var b strings.Builder
	b.WriteString("\n## Controlled vocabulary\n\n")
	b.WriteString("| subject | relation |\n|---|---|\n")
	for _, p := range vocabulary(all) {
		fmt.Fprintf(&b, "| %s | %s |\n", p.subject, p.relation)
	}
	return b.String()
}

func factLines(list []facts.Fact) []string {
	lines := make([]string, 0, len(list))
	for _, f := range list {
		tag := ""
		if f.Status == facts.StatusProposed {
			tag = "[proposed] "
		}
		lines = append(lines, fmt.Sprintf("- `%s` — %s%s\n", f.ID, tag, f.Title))
	}
	return lines
}

func droppedFooter(dropped int) string {
	if dropped == 0 {
		return ""
	}
	return fmt.Sprintf("\n_%d more fact(s) not shown to keep this small — use `bin/regesto-search`._\n", dropped)
}

// factBlock renders one scope's one-line entries, spending at most budget
// bytes and reporting how many entries it had to drop.
// A negative budget is unlimited. The heading is always retained, matching the
// header's discoverability guarantee even for an impractically tiny cap.
func factBlock(heading string, list []facts.Fact, budget int) (string, int) {
	if len(list) == 0 {
		return fmt.Sprintf("\n## %s\n\n(none)\n", heading), 0
	}
	var b strings.Builder
	b.WriteString(factHeading(heading, list))
	dropped := 0
	for _, line := range factLines(list) {
		if budget >= 0 && b.Len()+len(line) > budget {
			dropped++
			continue
		}
		b.WriteString(line)
	}
	return b.String(), dropped
}

func factHeading(heading string, list []facts.Fact) string {
	if len(list) == 0 {
		return fmt.Sprintf("\n## %s\n\n(none)\n", heading)
	}
	return fmt.Sprintf("\n## %s (%d)\n\n", heading, len(list))
}
