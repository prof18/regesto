// Package search implements `regesto search` (PLAN 1.a): query
// knowledge/facts/ by subject, relation, scope, and free text, hiding
// superseded claims unless history is requested and tagging proposed ones.
package search

import (
	"fmt"
	"strings"

	"github.com/prof18/regesto/internal/facts"
)

type Query struct {
	Subject  string
	Relation string
	// Scope is `global`, `project:<name>`, or a bare project name
	// (shorthand for project:<name>).
	Scope string
	// Terms are free-text terms; all must match (case-insensitive
	// substring over id, title, subject, relation, scope, topics, body).
	Terms []string
	// History includes status: superseded claims, which are hidden by
	// default per SCHEMA.md's status lifecycle.
	History bool
}

// Run filters the given facts. Input order (path order from LoadAll) is
// preserved, so output is deterministic.
func Run(all []facts.Fact, q Query) []facts.Fact {
	var out []facts.Fact
	for _, f := range all {
		if f.Status == facts.StatusSuperseded && !q.History {
			continue
		}
		if q.Subject != "" && !strings.EqualFold(f.Subject, q.Subject) {
			continue
		}
		if q.Relation != "" && !strings.EqualFold(f.Relation, q.Relation) {
			continue
		}
		if q.Scope != "" && !scopeMatches(f.Scope, q.Scope) {
			continue
		}
		if !termsMatch(f, q.Terms) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// FormatLine renders one result compactly — id, title, path — the shape
// that gets injected into agent context, where every token counts.
// Proposed claims are tagged so an agent can see it must caveat reliance.
func FormatLine(f facts.Fact) string {
	tag := ""
	if f.Status == facts.StatusProposed {
		tag = "[proposed] "
	}
	return fmt.Sprintf("%s\t%s%s\t%s", f.ID, tag, f.Title, f.RelPath)
}

func scopeMatches(factScope, want string) bool {
	if strings.EqualFold(factScope, want) {
		return true
	}
	// Bare project name as shorthand: `--scope aurora` ≡ `--scope project:aurora`.
	if want != "global" && !strings.Contains(want, ":") {
		return strings.EqualFold(factScope, "project:"+want)
	}
	return false
}

func termsMatch(f facts.Fact, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	haystack := strings.ToLower(strings.Join(append([]string{
		f.ID, f.Title, f.Subject, f.Relation, f.Scope, f.Body,
	}, f.Topics...), "\n"))
	for _, term := range terms {
		if !strings.Contains(haystack, strings.ToLower(term)) {
			return false
		}
	}
	return true
}
