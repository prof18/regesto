// Package lint validates knowledge/facts/ against SCHEMA.md and reconciles
// contradicting claims (PLAN 2.c, DESIGN §6).
//
// Two rules govern everything here:
//
//   - Nothing is ever deleted. A claim that loses becomes status: superseded and
//     stays on disk with its body intact.
//   - Nothing is silent. Every supersession, every flip, every skipped review is
//     reported, because a wrongly-merged pair is only catchable if you see it
//     happen (SCHEMA.md, Superseding rule 4).
package lint

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/prof18/regesto/internal/facts"
)

type Severity int

const (
	// Warn is drift worth fixing that does not threaten the model's integrity.
	Warn Severity = iota
	// Error breaks a SCHEMA rule; the store is not conformant until fixed.
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warn"
}

// Finding is one problem with one fact.
type Finding struct {
	Severity Severity
	ID       string
	Path     string
	Message  string
}

// Action is a mechanical change reconciliation made, or would make.
type Action struct {
	Kind    string // supersede | flip | review
	Path    string
	ID      string
	Message string
	Updates map[string]string
}

// Report is the run summary. Lint's contract is that this is complete: if
// something changed, it is in here.
type Report struct {
	Checked  int
	Findings []Finding
	Actions  []Action
	// Reviews are agent-vs-human contests that lint refuses to settle. They
	// are re-reported on every pass until a human resolves them.
	Reviews []string
	// NearDuplicates are vocabulary pairs close enough to be the same term
	// spelled two ways — the failure mode that makes contradiction
	// undetectable.
	NearDuplicates []string
	Due            []string
}

func (r *Report) errorf(f facts.Fact, format string, a ...any) {
	r.Findings = append(r.Findings, Finding{Error, f.ID, f.RelPath, fmt.Sprintf(format, a...)})
}

func (r *Report) warnf(f facts.Fact, format string, a ...any) {
	r.Findings = append(r.Findings, Finding{Warn, f.ID, f.RelPath, fmt.Sprintf(format, a...)})
}

// Errors reports whether anything blocking was found.
func (r *Report) Errors() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == Error {
			n++
		}
	}
	return n
}

var (
	validTypes    = map[string]string{"decision": "dec-", "preference": "pref-", "fact": "fact-", "pattern": "pat-"}
	validStatuses = map[string]bool{facts.StatusActive: true, facts.StatusProposed: true, facts.StatusSuperseded: true}
)

// Run validates every fact and works out the reconciliation actions. It makes no
// changes; the caller applies Report.Actions if asked to.
func Run(all []facts.Fact, now time.Time) *Report {
	r := &Report{Checked: len(all)}
	byID := make(map[string]facts.Fact, len(all))

	for _, f := range all {
		validate(r, f, byID, now)
		byID[f.ID] = f
	}

	// Dangling supersedes: a reference to a claim that is not in the store
	// means the history is broken and the standing flip rule cannot fire.
	for _, f := range all {
		if f.Supersedes != "" {
			if _, ok := byID[f.Supersedes]; !ok {
				r.errorf(f, "supersedes: %s — no such claim in the store", f.Supersedes)
			}
		}
	}

	reconcile(r, all, byID, now)
	vocabulary(r, all)
	reviewDue(r, all, now)
	return r
}

func validate(r *Report, f facts.Fact, seen map[string]facts.Fact, now time.Time) {
	if prev, dup := seen[f.ID]; dup {
		r.errorf(f, "duplicate id — already used by %s", prev.RelPath)
	}
	if f.SchemaVersion != "1" {
		r.errorf(f, "schema_version is %q, expected 1", f.SchemaVersion)
	}
	prefix, ok := validTypes[f.Type]
	if !ok {
		r.errorf(f, "type %q is not one of decision/preference/fact/pattern", f.Type)
	} else if !strings.HasPrefix(f.ID, prefix) {
		r.errorf(f, "type %s requires an id starting %q", f.Type, prefix)
	}
	if !validStatuses[f.Status] {
		r.errorf(f, "status %q is not one of active/proposed/superseded", f.Status)
	}
	if n := len([]rune(f.Title)); n > 80 {
		r.warnf(f, "title is %d chars, SCHEMA says ≤80", n)
	}
	if f.Source == "" {
		r.errorf(f, "source is required — provenance is what makes trust rules work")
	}

	// The file must live where its scope says, or scoped search silently
	// misses it.
	wantDir := "knowledge/facts/global/"
	if p := f.ProjectName(); p != "" {
		wantDir = "knowledge/facts/projects/" + p + "/"
	} else if f.Scope != "global" {
		r.errorf(f, "scope %q must be `global` or `project:<name>`", f.Scope)
		wantDir = ""
	}
	if wantDir != "" {
		if !strings.HasPrefix(f.RelPath, wantDir) {
			r.errorf(f, "scope %s expects the file under %s", f.Scope, wantDir)
		}
		if want := wantDir + f.ID + ".md"; f.RelPath != want {
			r.warnf(f, "filename does not match id — expected %s", want)
		}
	}

	created := checkTime(r, f, "created", f.Created, now)
	modified := checkTime(r, f, "modified", f.Modified, now)
	if !created.IsZero() && !modified.IsZero() && modified.Before(created) {
		r.errorf(f, "modified %s is before created %s", f.Modified, f.Created)
	}
}

// checkTime parses an ISO 8601 UTC stamp and flags the two mistakes seen in
// practice: a guessed date, and a stamp in the future.
func checkTime(r *Report, f facts.Fact, field, value string, now time.Time) time.Time {
	if value == "" {
		r.errorf(f, "%s is required", field)
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		r.errorf(f, "%s %q is not ISO 8601 UTC (want 2006-01-02T15:04:05Z)", field, value)
		return time.Time{}
	}
	if t.After(now.Add(24 * time.Hour)) {
		r.errorf(f, "%s %s is in the future", field, value)
	}
	// Midnight precision is only a smell on `modified`, which is stamped at
	// write time and so should carry a real clock reading. On `created` it is
	// legitimate: a fact migrated from an older note carries that note's date,
	// which is known only to the day (PLAN 0.c).
	if field == "modified" && t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		r.warnf(f, "modified %s has midnight precision — looks guessed rather than stamped; run `date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ`", value)
	}
	return t
}

// reconcile implements SCHEMA.md's Superseding section.
func reconcile(r *Report, all []facts.Fact, byID map[string]facts.Fact, now time.Time) {
	stamp := now.UTC().Format(time.RFC3339)

	// Standing rule: anything named in an *active* claim's supersedes: is
	// retired. This is the mechanical half of the review workflow — a human
	// accepts a proposed winner with a one-field edit and lint does the rest.
	retired := map[string]bool{}
	for _, f := range all {
		if f.Status != facts.StatusActive || f.Supersedes == "" {
			continue
		}
		old, ok := byID[f.Supersedes]
		if !ok || old.Status == facts.StatusSuperseded {
			continue
		}
		retired[old.ID] = true
		r.Actions = append(r.Actions, Action{
			Kind: "flip", Path: old.RelPath, ID: old.ID,
			Message: fmt.Sprintf("superseded by active %s", f.ID),
			Updates: map[string]string{"status": facts.StatusSuperseded, "modified": stamp},
		})
	}

	// Contests are decided only among active and proposed claims. A superseded
	// claim is out permanently: its modified was bumped when it lost, so
	// recency alone would flip old winners back on a later pass.
	groups := map[[2]string][]facts.Fact{}
	for _, f := range all {
		if f.Status == facts.StatusSuperseded || retired[f.ID] {
			continue
		}
		key := [2]string{strings.ToLower(f.Subject), strings.ToLower(f.Relation)}
		groups[key] = append(groups[key], f)
	}

	for _, key := range sortedPairKeys(groups) {
		claims := groups[key]
		if len(claims) < 2 {
			continue
		}
		sort.Slice(claims, func(i, j int) bool {
			ti, _ := time.Parse(time.RFC3339, claims[i].Modified)
			tj, _ := time.Parse(time.RFC3339, claims[j].Modified)
			if ti.Equal(tj) {
				return claims[i].ID < claims[j].ID
			}
			return ti.After(tj)
		})
		winner := claims[0]
		for _, loser := range claims[1:] {
			// The trust boundary: an agent claim never auto-supersedes a
			// human one. It lands as proposed with the intent recorded, and
			// a human decides.
			if loser.Source == "human" && winner.Source != "human" {
				r.Reviews = append(r.Reviews, fmt.Sprintf(
					"%s (%s) contests human claim %s on (%s, %s) — left for review; human stays active",
					winner.ID, winner.Source, loser.ID, key[0], key[1]))
				updates := map[string]string{}
				if winner.Status != facts.StatusProposed {
					updates["status"] = facts.StatusProposed
				}
				if winner.Supersedes != loser.ID {
					updates["supersedes"] = loser.ID
				}
				if len(updates) > 0 {
					r.Actions = append(r.Actions, Action{
						Kind: "review", Path: winner.RelPath, ID: winner.ID,
						Message: fmt.Sprintf("record intent to supersede human claim %s, pending review", loser.ID),
						Updates: updates,
					})
				}
				continue
			}

			r.Actions = append(r.Actions,
				Action{
					Kind: "supersede", Path: loser.RelPath, ID: loser.ID,
					Message: fmt.Sprintf("loses (%s, %s) to newer %s", key[0], key[1], winner.ID),
					Updates: map[string]string{"status": facts.StatusSuperseded, "modified": stamp},
				},
				Action{
					Kind: "supersede", Path: winner.RelPath, ID: winner.ID,
					Message: fmt.Sprintf("wins (%s, %s), supersedes %s", key[0], key[1], loser.ID),
					Updates: map[string]string{"supersedes": loser.ID},
				})
		}
	}
}

// vocabulary reports pairs close enough to be one term spelled two ways. This is
// the failure that makes contradiction undetectable, so it is reported for a
// human to merge rather than guessed at.
func vocabulary(r *Report, all []facts.Fact) {
	type pair struct{ subject, relation string }
	seen := map[pair]bool{}
	var pairs []pair
	for _, f := range all {
		p := pair{f.Subject, f.Relation}
		if !seen[p] {
			seen[p] = true
			pairs = append(pairs, p)
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].subject != pairs[j].subject {
			return pairs[i].subject < pairs[j].subject
		}
		return pairs[i].relation < pairs[j].relation
	})
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			a, b := pairs[i], pairs[j]
			if a.subject == b.subject && a.relation != b.relation && near(a.relation, b.relation) {
				r.NearDuplicates = append(r.NearDuplicates, fmt.Sprintf(
					"(%s, %s) vs (%s, %s) — same subject, near-identical relation", a.subject, a.relation, b.subject, b.relation))
			}
			if a.relation == b.relation && a.subject != b.subject && near(a.subject, b.subject) {
				r.NearDuplicates = append(r.NearDuplicates, fmt.Sprintf(
					"(%s, %s) vs (%s, %s) — same relation, near-identical subject", a.subject, a.relation, b.subject, b.relation))
			}
		}
	}
}

func reviewDue(r *Report, all []facts.Fact, now time.Time) {
	for _, f := range all {
		if f.ReviewAfter == "" || f.Status == facts.StatusSuperseded {
			continue
		}
		t, err := time.Parse("2006-01-02", strings.TrimSpace(f.ReviewAfter))
		if err != nil {
			if t, err = time.Parse(time.RFC3339, f.ReviewAfter); err != nil {
				r.warnf(f, "review_after %q is not a date", f.ReviewAfter)
				continue
			}
		}
		if now.After(t) {
			r.Due = append(r.Due, fmt.Sprintf("%s — review_after %s has passed", f.ID, f.ReviewAfter))
		}
	}
}

// near reports whether two vocabulary terms are within one edit, or one is the
// other plus a suffix like -s. Deliberately conservative: this only ever
// produces a report for a human, so a false positive costs a glance.
func near(a, b string) bool {
	if a == b {
		return false
	}
	if strings.HasPrefix(a, b) || strings.HasPrefix(b, a) {
		return true
	}
	return editDistance(a, b) <= 2 && len(a) > 4 && len(b) > 4
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func sortedPairKeys(m map[[2]string][]facts.Fact) [][2]string {
	keys := make([][2]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	return keys
}
