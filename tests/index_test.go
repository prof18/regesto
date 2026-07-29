// Tests for PLAN 1.b acceptance: INDEX.md regenerated from knowledge/facts/
// with scope sections (global first), the controlled vocabulary table,
// topics in use, and counts by status; knowledge/topics/ regenerated from
// the topics: frontmatter lists.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regesto/internal/facts"
	"regesto/internal/index"
)

func buildFixtureIndex(t *testing.T) index.Result {
	t.Helper()
	return index.Build(loadFixtures(t))
}

func TestIndexHasScopeSectionsGlobalFirst(t *testing.T) {
	r := buildFixtureIndex(t)

	globalPos := strings.Index(r.IndexMD, "## Global")
	projectPos := strings.Index(r.IndexMD, "## Project: aurora")
	if globalPos == -1 || projectPos == -1 {
		t.Fatalf("missing scope sections:\n%s", r.IndexMD)
	}
	if globalPos > projectPos {
		t.Errorf("global section must come before project sections")
	}

	// Listings carry id, title, and path; proposed is tagged; superseded
	// facts are not listed.
	for _, want := range []string{
		"- `pref-git-commit-style` — Commit subjects are imperative, lower-case, no trailing period (knowledge/facts/global/pref-git-commit-style.md)",
		"- `pref-git-commit-style-emoji` — [proposed] Commit subjects start with a gitmoji (knowledge/facts/global/pref-git-commit-style-emoji.md)",
		"- `dec-http-port-8080` — Aurora dev server listens on port 8080 (knowledge/facts/projects/aurora/dec-http-port-8080.md)",
	} {
		if !strings.Contains(r.IndexMD, want) {
			t.Errorf("INDEX.md missing listing line %q", want)
		}
	}
	if strings.Contains(r.IndexMD, "- `dec-http-port` —") {
		t.Errorf("superseded fact listed in INDEX.md scope sections")
	}
}

func TestIndexControlledVocabulary(t *testing.T) {
	r := buildFixtureIndex(t)
	for _, row := range []string{
		"| build | required-flags | 1 |",
		"| git | commit-style | 2 |",
		"| http-server | port | 2 |",
	} {
		if !strings.Contains(r.IndexMD, row) {
			t.Errorf("vocabulary table missing row %q\n%s", row, r.IndexMD)
		}
	}
}

func TestIndexStatusCounts(t *testing.T) {
	r := buildFixtureIndex(t)
	for _, row := range []string{
		"| active | 4 |",
		"| proposed | 1 |",
		"| superseded | 1 |",
	} {
		if !strings.Contains(r.IndexMD, row) {
			t.Errorf("status counts missing row %q\n%s", row, r.IndexMD)
		}
	}
}

func TestIndexTopicsInUse(t *testing.T) {
	r := buildFixtureIndex(t)
	// aurora: dec-http-port-8080 + dec-quoted-title + fact-build-flag
	// (superseded dec-http-port excluded); git: both commit-style claims;
	// tooling: fact-build-flag.
	for _, line := range []string{
		"- [[aurora]] — 3 fact(s)",
		"- [[git]] — 2 fact(s)",
		"- [[tooling]] — 1 fact(s)",
	} {
		if !strings.Contains(r.IndexMD, line) {
			t.Errorf("topics section missing %q\n%s", line, r.IndexMD)
		}
	}
}

func TestTopicPagesRegenerated(t *testing.T) {
	r := buildFixtureIndex(t)
	aurora, ok := r.TopicPages["aurora"]
	if !ok {
		t.Fatalf("no aurora topic page; got %v", keysOf(r.TopicPages))
	}
	for _, want := range []string{
		"- [[dec-http-port-8080]] — Aurora dev server listens on port 8080 (project:aurora)",
		"- [[fact-build-flag]] — Aurora release builds require the neon build tag (project:aurora)",
	} {
		if !strings.Contains(aurora, want) {
			t.Errorf("aurora topic page missing %q\n%s", want, aurora)
		}
	}
	if strings.Contains(aurora, "dec-http-port]]") {
		t.Errorf("superseded fact linked on topic page:\n%s", aurora)
	}
	if _, ok := r.TopicPages["git"]; !ok {
		t.Errorf("git topic page missing")
	}
}

// Write must fully regenerate: INDEX.md lands at the root and a stale,
// hand-created page in knowledge/topics/ is removed (generated artifacts
// are caches — SCHEMA.md).
func TestWriteRegeneratesAndRemovesStalePages(t *testing.T) {
	root := t.TempDir()
	copyTree(t, fixtureRoot(t), root)
	topicsDir := filepath.Join(root, "knowledge", "topics")
	if err := os.MkdirAll(topicsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(topicsDir, "stale-topic.md")
	if err := os.WriteFile(stale, []byte("# leftover"), 0o644); err != nil {
		t.Fatal(err)
	}

	all, err := facts.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Write(root, index.Build(all)); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "INDEX.md")); err != nil {
		t.Errorf("INDEX.md not written at KB root: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale topic page survived regeneration")
	}
	for _, page := range []string{"aurora.md", "git.md", "tooling.md"} {
		if _, err := os.Stat(filepath.Join(topicsDir, page)); err != nil {
			t.Errorf("topic page %s not written: %v", page, err)
		}
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copyTree: %v", err)
	}
}

func keysOf[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
