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

	// The header is never truncated — a payload that lost the search
	// instructions would be worse than no payload at all.
	budget := opts.MaxBytes - b.Len()

	globalBlock, globalDropped := factBlock("Global", globals, &budget)
	projectBlock, projectDropped := factBlock("Project: "+opts.Project, project, &budget)

	b.WriteString(globalBlock)
	if opts.Project != "" {
		b.WriteString(projectBlock)
	}

	if opts.Vocabulary {
		var v strings.Builder
		v.WriteString("\n## Controlled vocabulary\n\n")
		v.WriteString("| subject | relation |\n|---|---|\n")
		for _, p := range vocabulary(all) {
			fmt.Fprintf(&v, "| %s | %s |\n", p.subject, p.relation)
		}
		if opts.MaxBytes <= 0 || v.Len() <= budget {
			b.WriteString(v.String())
		}
	}

	if globalDropped+projectDropped > 0 {
		fmt.Fprintf(&b, "\n_%d more fact(s) not shown to keep this small — use `bin/regesto-search`._\n",
			globalDropped+projectDropped)
	}
	return b.String()
}

// factBlock renders one scope's one-line entries, spending at most *budget
// bytes and reporting how many entries it had to drop.
func factBlock(heading string, list []facts.Fact, budget *int) (string, int) {
	if len(list) == 0 {
		return fmt.Sprintf("\n## %s\n\n(none)\n", heading), 0
	}
	head := fmt.Sprintf("\n## %s (%d)\n\n", heading, len(list))
	var b strings.Builder
	b.WriteString(head)
	dropped := 0
	for _, f := range list {
		tag := ""
		if f.Status == facts.StatusProposed {
			tag = "[proposed] "
		}
		line := fmt.Sprintf("- `%s` — %s%s\n", f.ID, tag, f.Title)
		if *budget > 0 && b.Len()+len(line) > *budget {
			dropped++
			continue
		}
		b.WriteString(line)
	}
	*budget -= b.Len()
	return b.String(), dropped
}
