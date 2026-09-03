// Tests for PLAN 2.c / SCHEMA.md's Superseding rules. The trust boundary and
// the "superseded never re-enters" rule are the two that would corrupt the store
// quietly if they regressed, so they are pinned hardest.
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/lint"
)

var lintNow = time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)

// fact builds a valid fact; each test perturbs only what it is about.
func fact(id, subject, relation, status, source, modified string) facts.Fact {
	f := facts.Fact{
		SchemaVersion: "1",
		ID:            id,
		Title:         "A claim",
		Type:          "decision",
		Scope:         "global",
		Subject:       subject,
		Relation:      relation,
		Status:        status,
		Source:        source,
		Created:       "2026-07-01T09:00:00Z",
		Modified:      modified,
		RelPath:       "knowledge/facts/global/" + id + ".md",
	}
	switch {
	case strings.HasPrefix(id, "pref-"):
		f.Type = "preference"
	case strings.HasPrefix(id, "fact-"):
		f.Type = "fact"
	case strings.HasPrefix(id, "pat-"):
		f.Type = "pattern"
	}
	return f
}

func findAction(r *lint.Report, id, kind string) *lint.Action {
	for i := range r.Actions {
		if r.Actions[i].ID == id && r.Actions[i].Kind == kind {
			return &r.Actions[i]
		}
	}
	return nil
}

func messages(r *lint.Report) string {
	var b strings.Builder
	for _, f := range r.Findings {
		b.WriteString(f.Severity.String() + " " + f.ID + ": " + f.Message + "\n")
	}
	return b.String()
}

func TestCleanStoreHasNoFindings(t *testing.T) {
	r := lint.Run([]facts.Fact{
		fact("dec-a", "http-server", "port", facts.StatusActive, "human", "2026-07-02T10:00:00Z"),
		fact("pref-b", "gradle", "console-flags", facts.StatusActive, "claude@studio", "2026-07-02T10:00:00Z"),
	}, lintNow)
	if r.Errors() != 0 || len(r.Actions) != 0 {
		t.Fatalf("clean store produced findings/actions:\n%s%v", messages(r), r.Actions)
	}
}

func TestTitleLengthFailsAboveHardLimit(t *testing.T) {
	atMaximum := fact("dec-title-at-maximum", "titles", "at-maximum", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	atMaximum.Title = strings.Repeat("é", facts.TitleMaxLength)
	overMaximum := fact("dec-title-over-maximum", "titles", "over-maximum", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	overMaximum.Title = strings.Repeat("é", facts.TitleMaxLength+1)

	r := lint.Run([]facts.Fact{atMaximum, overMaximum}, lintNow)
	got := messages(r)
	if strings.Contains(got, atMaximum.ID) {
		t.Fatalf("title at hard maximum produced a finding:\n%s", got)
	}
	if !strings.Contains(got, "error "+overMaximum.ID+": title is 101 chars") || r.Errors() != 1 {
		t.Fatalf("title above hard maximum did not fail lint:\n%s", got)
	}
}

func TestLintCommandFailsForTitleAboveHardLimit(t *testing.T) {
	cfg := normalizeInstance(t)
	path := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "dec-overlong-title.md")
	body := "---\nschema_version: 1\nid: dec-overlong-title\ntitle: " + strings.Repeat("é", facts.TitleMaxLength+1) +
		"\ntype: decision\nscope: global\nsubject: titles\nrelation: cli-limit\nstatus: active\n" +
		"source: human\ncreated: 2026-07-02T10:00:00Z\nmodified: 2026-07-02T10:00:00Z\n---\n\nA claim.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./cmd/regesto", "--config", cfg.Path, "lint", "--quiet")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "GOTELEMETRY=off", "GOCACHE="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("lint accepted a title above the hard limit:\n%s", out)
	}
	if !strings.Contains(string(out), "error "+filepath.ToSlash(strings.TrimPrefix(path, cfg.KBRoot+string(filepath.Separator)))+": title is 101 chars") {
		t.Fatalf("lint did not report the overlong title clearly: %v\n%s", err, out)
	}
}

func TestNewerClaimSupersedesOlder(t *testing.T) {
	old := fact("dec-port-8000", "http-server", "port", facts.StatusActive, "claude@studio", "2026-07-02T10:00:00Z")
	new := fact("dec-port-8080", "http-server", "port", facts.StatusActive, "claude@studio", "2026-07-20T10:00:00Z")
	r := lint.Run([]facts.Fact{old, new}, lintNow)

	loser := findAction(r, "dec-port-8000", "supersede")
	if loser == nil {
		t.Fatalf("older claim was not superseded; actions=%v", r.Actions)
	}
	if loser.Updates["status"] != facts.StatusSuperseded {
		t.Errorf("loser status = %q, want superseded", loser.Updates["status"])
	}
	if loser.Updates["modified"] == "" {
		t.Error("loser's modified must be bumped when it loses")
	}
	winner := findAction(r, "dec-port-8080", "supersede")
	if winner == nil || winner.Updates["supersedes"] != "dec-port-8000" {
		t.Errorf("winner did not record supersedes: %v", winner)
	}
}

// SCHEMA.md, Superseding rule 2. This is the rule that protects the user's own
// assertions from being quietly overwritten by an agent.
func TestAgentNeverAutoSupersedesHuman(t *testing.T) {
	human := fact("dec-human", "deploy", "target", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	agent := fact("dec-agent", "deploy", "target", facts.StatusActive, "codex@studio", "2026-07-20T10:00:00Z")
	r := lint.Run([]facts.Fact{human, agent}, lintNow)

	if a := findAction(r, "dec-human", "supersede"); a != nil {
		t.Fatalf("human claim was auto-superseded by an agent claim: %v", a)
	}
	review := findAction(r, "dec-agent", "review")
	if review == nil {
		t.Fatalf("agent claim should land as proposed with intent recorded; actions=%v", r.Actions)
	}
	if review.Updates["status"] != facts.StatusProposed {
		t.Errorf("challenger status = %q, want proposed", review.Updates["status"])
	}
	if review.Updates["supersedes"] != "dec-human" {
		t.Errorf("challenger should record supersedes: dec-human, got %q", review.Updates["supersedes"])
	}
	if len(r.Reviews) != 1 {
		t.Errorf("the pending review must be reported, got %v", r.Reviews)
	}
}

// A human accepting a proposal is a one-field edit; lint retires the old claim.
func TestActiveSupersedesFlipsNamedClaim(t *testing.T) {
	old := fact("dec-old", "deploy", "target", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	accepted := fact("dec-new", "deploy", "target", facts.StatusActive, "codex@studio", "2026-07-20T10:00:00Z")
	accepted.Supersedes = "dec-old"

	r := lint.Run([]facts.Fact{old, accepted}, lintNow)
	flip := findAction(r, "dec-old", "flip")
	if flip == nil {
		t.Fatalf("claim named by an active supersedes: was not retired; actions=%v", r.Actions)
	}
	if flip.Updates["status"] != facts.StatusSuperseded {
		t.Errorf("flip status = %q, want superseded", flip.Updates["status"])
	}
	// Having been retired, it must not also be treated as a live contender.
	if a := findAction(r, "dec-old", "review"); a != nil {
		t.Errorf("retired claim re-entered reconciliation as a review: %v", a)
	}
}

// SCHEMA.md, Superseding rule 1: a superseded claim's modified was bumped when
// it lost, so recency alone would otherwise flip old winners back on a re-run.
func TestSupersededClaimNeverReEnters(t *testing.T) {
	loser := fact("dec-old", "http-server", "port", facts.StatusSuperseded, "claude@studio", "2026-07-27T10:00:00Z")
	winner := fact("dec-new", "http-server", "port", facts.StatusActive, "claude@studio", "2026-07-20T10:00:00Z")

	r := lint.Run([]facts.Fact{loser, winner}, lintNow)
	if len(r.Actions) != 0 {
		t.Fatalf("a superseded claim with a newer modified re-entered reconciliation: %v", r.Actions)
	}
}

func TestValidationCatchesSchemaBreaks(t *testing.T) {
	dup := fact("dec-a", "s", "r", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	dup2 := fact("dec-a", "s2", "r2", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	dup2.RelPath = "knowledge/facts/global/other.md"

	badPrefix := fact("dec-mismatch", "s3", "r3", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	badPrefix.Type = "preference"

	dangling := fact("dec-d", "s4", "r4", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	dangling.Supersedes = "dec-does-not-exist"

	backwards := fact("dec-e", "s5", "r5", facts.StatusActive, "human", "2026-06-01T10:00:00Z")

	future := fact("dec-f", "s6", "r6", facts.StatusActive, "human", "2027-01-01T10:00:00Z")

	misfiled := fact("dec-g", "s7", "r7", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	misfiled.Scope = "project:aurora"

	r := lint.Run([]facts.Fact{dup, dup2, badPrefix, dangling, backwards, future, misfiled}, lintNow)
	got := messages(r)
	for _, want := range []string{
		"duplicate id",
		"requires an id starting",
		"no such claim in the store",
		"is before created",
		"is in the future",
		"expects the file under",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing expected finding %q in:\n%s", want, got)
		}
	}
}

func TestNearDuplicateVocabularyReported(t *testing.T) {
	a := fact("dec-a", "console", "cli-flags", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	b := fact("dec-b", "console", "cli-flag", facts.StatusActive, "human", "2026-07-03T10:00:00Z")
	r := lint.Run([]facts.Fact{a, b}, lintNow)
	if len(r.NearDuplicates) == 0 {
		t.Fatal("near-identical relations should be reported for manual merge")
	}
	// Reported, never merged automatically — merging is a judgement call.
	if len(r.Actions) != 0 {
		t.Errorf("near-duplicates must not be auto-merged: %v", r.Actions)
	}
}

func TestReviewAfterDueIsFlagged(t *testing.T) {
	f := fact("dec-a", "s", "r", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	f.ReviewAfter = "2026-07-01"
	notYet := fact("dec-b", "s2", "r2", facts.StatusActive, "human", "2026-07-02T10:00:00Z")
	notYet.ReviewAfter = "2027-01-01"

	r := lint.Run([]facts.Fact{f, notYet}, lintNow)
	if len(r.Due) != 1 || !strings.Contains(r.Due[0], "dec-a") {
		t.Errorf("expected only dec-a to be due, got %v", r.Due)
	}
}

// SetFields must be surgical: the body and untouched fields survive byte-exact.
func TestSetFieldsPreservesEverythingElse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dec-a.md")
	original := `---
schema_version: 1
id: dec-a
title: "A claim: with a colon"
type: decision
scope: global
subject: s
relation: r
topics: [x, y]
status: active
source: human
created:  2026-07-01T09:00:00Z
modified: 2026-07-01T09:00:00Z
custom_field: keep me
---

The body, which must never be edited.

**Why:** because SCHEMA says so.
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := facts.SetFields(path, map[string]string{
		"status":     facts.StatusSuperseded,
		"modified":   "2026-07-28T12:00:00Z",
		"supersedes": "dec-b",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)

	for _, want := range []string{
		"status: superseded",
		"modified: 2026-07-28T12:00:00Z",
		"supersedes: dec-b",
		`title: "A claim: with a colon"`,
		"custom_field: keep me",
		"topics: [x, y]",
		"The body, which must never be edited.",
		"**Why:** because SCHEMA says so.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q after edit:\n%s", want, got)
		}
	}
	if strings.Contains(got, "status: active") {
		t.Error("old status value survived")
	}
	// created keeps its alignment padding rather than being reflowed.
	if !strings.Contains(got, "created:  2026-07-01T09:00:00Z") {
		t.Error("created line was disturbed")
	}
	// The file must still parse.
	if _, err := facts.ParseFile(path); err != nil {
		t.Errorf("edited file no longer parses: %v", err)
	}
}
