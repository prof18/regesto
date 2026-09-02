// Tests for PLAN 1.a acceptance: query by subject/relation/scope/free text,
// superseded hidden unless --history, proposed tagged, compact output.
package tests

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/search"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(thisFile), "fixtures", "kb")
}

func loadFixtures(t *testing.T) []facts.Fact {
	t.Helper()
	all, err := facts.LoadAll(fixtureRoot(t))
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 6 {
		t.Fatalf("expected 6 fixture facts, got %d", len(all))
	}
	return all
}

func ids(results []facts.Fact) []string {
	var out []string
	for _, f := range results {
		out = append(out, f.ID)
	}
	return out
}

func assertIDs(t *testing.T, got []facts.Fact, want ...string) {
	t.Helper()
	g := strings.Join(ids(got), ",")
	w := strings.Join(want, ",")
	if g != w {
		t.Fatalf("got ids [%s], want [%s]", g, w)
	}
}

func TestSubjectFilterHidesSuperseded(t *testing.T) {
	all := loadFixtures(t)
	// Two facts share (http-server, port); the superseded loser must not
	// appear without --history.
	got := search.Run(all, search.Query{Subject: "http-server"})
	assertIDs(t, got, "dec-http-port-8080")
}

func TestHistoryIncludesSuperseded(t *testing.T) {
	all := loadFixtures(t)
	got := search.Run(all, search.Query{Subject: "http-server", History: true})
	assertIDs(t, got, "dec-http-port-8080", "dec-http-port")
}

func TestRelationFilter(t *testing.T) {
	all := loadFixtures(t)
	got := search.Run(all, search.Query{Relation: "required-flags"})
	assertIDs(t, got, "fact-build-flag")
}

func TestScopeFilter(t *testing.T) {
	all := loadFixtures(t)

	got := search.Run(all, search.Query{Scope: "project:aurora"})
	assertIDs(t, got, "dec-http-port-8080", "dec-quoted-title", "fact-build-flag")

	// Bare project name is shorthand for project:<name>.
	got = search.Run(all, search.Query{Scope: "aurora"})
	assertIDs(t, got, "dec-http-port-8080", "dec-quoted-title", "fact-build-flag")

	got = search.Run(all, search.Query{Scope: "global"})
	assertIDs(t, got, "pref-git-commit-style-emoji", "pref-git-commit-style")
}

func TestFreeTextSearch(t *testing.T) {
	all := loadFixtures(t)

	// Matches body text, case-insensitively.
	got := search.Run(all, search.Query{Terms: []string{"NEON"}})
	assertIDs(t, got, "fact-build-flag")

	// Multiple terms AND together.
	got = search.Run(all, search.Query{Terms: []string{"8080", "metrics"}})
	assertIDs(t, got, "dec-http-port-8080")

	// Free text combines with field filters.
	got = search.Run(all, search.Query{Scope: "global", Terms: []string{"imperative"}})
	assertIDs(t, got, "pref-git-commit-style")
}

func TestProposedFactsAreTagged(t *testing.T) {
	all := loadFixtures(t)
	got := search.Run(all, search.Query{Subject: "git"})
	assertIDs(t, got, "pref-git-commit-style-emoji", "pref-git-commit-style")

	var proposed, active string
	for _, f := range got {
		line := search.FormatLine(f)
		switch f.ID {
		case "pref-git-commit-style-emoji":
			proposed = line
		case "pref-git-commit-style":
			active = line
		}
	}
	if !strings.Contains(proposed, "[proposed]") {
		t.Errorf("proposed fact not tagged: %q", proposed)
	}
	if strings.Contains(active, "[proposed]") {
		t.Errorf("active fact wrongly tagged: %q", active)
	}
}

func TestCompactOutputIsIDTitlePath(t *testing.T) {
	all := loadFixtures(t)
	got := search.Run(all, search.Query{Subject: "http-server"})
	line := search.FormatLine(got[0])
	want := "dec-http-port-8080\tAurora dev server listens on port 8080\tknowledge/facts/projects/aurora/dec-http-port-8080.md"
	if line != want {
		t.Errorf("compact line mismatch:\n got %q\nwant %q", line, want)
	}
}

// A title containing a colon must be YAML-quoted; the quotes are syntax and
// must not survive into the title or the compact output.
func TestQuotedFrontmatterValueIsUnquoted(t *testing.T) {
	all := loadFixtures(t)
	got := search.Run(all, search.Query{Subject: "logging"})
	assertIDs(t, got, "dec-quoted-title")

	if want := "Aurora logging: structured JSON only"; got[0].Title != want {
		t.Errorf("title = %q, want %q", got[0].Title, want)
	}
	if line := search.FormatLine(got[0]); strings.Contains(line, `"`) {
		t.Errorf("quotes leaked into compact output: %q", line)
	}
}

func TestFixtureConfigLoads(t *testing.T) {
	cfg, err := config.Load(filepath.Join(fixtureRoot(t), "config.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Machine != "testbox" {
		t.Errorf("machine = %q, want testbox", cfg.Machine)
	}
	if len(cfg.Integrations) != 2 || cfg.Integrations[0] != "claude" {
		t.Errorf("integrations = %v", cfg.Integrations)
	}
	if cfg.Projects["aurora-2"] != "aurora" {
		t.Errorf("projects map = %v", cfg.Projects)
	}
	if cfg.KBRoot != fixtureRoot(t) {
		t.Errorf("kb_root = %q, want fixture root", cfg.KBRoot)
	}
}
