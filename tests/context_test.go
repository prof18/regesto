// Tests for PLAN 1.c: the SessionStart payload injects INDEX-style listings
// scoped to the current repo, maps a repo to its canonical project name, and
// stays small.
package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/index"
	"github.com/prof18/regesto/internal/project"
)

func TestContextInjectsGlobalAndProjectScopes(t *testing.T) {
	out := index.BuildContext(loadFixtures(t), index.ContextOptions{Project: "aurora"})

	// Search instructions must always be present — a payload without them is
	// worse than none.
	for _, want := range []string{"bin/regesto-search", "INDEX.md", "SCHEMA.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("payload missing pointer %q", want)
		}
	}
	if !strings.Contains(out, "## Global (2)") {
		t.Errorf("missing global section:\n%s", out)
	}
	if !strings.Contains(out, "## Project: aurora (3)") {
		t.Errorf("missing project section:\n%s", out)
	}
	// Titles are listed; fact bodies are not — they stay behind bin/regesto-search.
	if !strings.Contains(out, "Aurora dev server listens on port 8080") {
		t.Errorf("project fact title missing:\n%s", out)
	}
	if strings.Contains(out, "collided with the metrics exporter") {
		t.Errorf("fact body leaked into the payload:\n%s", out)
	}
	// Superseded stays out; proposed is tagged.
	if strings.Contains(out, "dec-http-port`") {
		t.Errorf("superseded fact leaked into the payload:\n%s", out)
	}
	if !strings.Contains(out, "[proposed] Commit subjects start with a gitmoji") {
		t.Errorf("proposed fact not tagged:\n%s", out)
	}
}

func TestContextOmitsOtherProjects(t *testing.T) {
	out := index.BuildContext(loadFixtures(t), index.ContextOptions{Project: "aurora"})
	if strings.Contains(out, "## Project: borealis") {
		t.Errorf("unrelated project section present:\n%s", out)
	}

	// No project resolved: global only, and no empty project heading.
	out = index.BuildContext(loadFixtures(t), index.ContextOptions{})
	if !strings.Contains(out, "## Global (2)") {
		t.Errorf("global section missing when no project:\n%s", out)
	}
	if strings.Contains(out, "## Project:") {
		t.Errorf("project heading rendered with no project:\n%s", out)
	}
}

func TestContextVocabularyIsOptIn(t *testing.T) {
	plain := index.BuildContext(loadFixtures(t), index.ContextOptions{Project: "aurora"})
	if strings.Contains(plain, "## Controlled vocabulary") {
		t.Errorf("vocabulary table included by default — it should be opt-in")
	}
	// The pair count is always reported, so the agent knows to go look.
	if !strings.Contains(plain, "pairs in use") {
		t.Errorf("vocabulary pair count missing:\n%s", plain)
	}

	withVocab := index.BuildContext(loadFixtures(t), index.ContextOptions{
		Project: "aurora", Vocabulary: true, MaxBytes: 0,
	})
	if !strings.Contains(withVocab, "## Controlled vocabulary") {
		t.Errorf("vocabulary table missing when requested:\n%s", withVocab)
	}
	if !strings.Contains(withVocab, "| http-server | port |") {
		t.Errorf("vocabulary row missing:\n%s", withVocab)
	}
}

// This payload lands in every session, so the cap is load-bearing.
func TestContextRespectsByteBudget(t *testing.T) {
	all := loadFixtures(t)

	full := index.BuildContext(all, index.ContextOptions{Project: "aurora", MaxBytes: 0})
	capped := index.BuildContext(all, index.ContextOptions{Project: "aurora", MaxBytes: 700})

	if len(capped) >= len(full) {
		t.Fatalf("cap had no effect: capped %d bytes, full %d", len(capped), len(full))
	}
	if len(capped) > 700 {
		t.Fatalf("capped payload is %d bytes, want at most 700", len(capped))
	}
	// The header survives truncation.
	for _, want := range []string{"bin/regesto-search", "SCHEMA.md"} {
		if !strings.Contains(capped, want) {
			t.Errorf("cap dropped the header pointer %q:\n%s", want, capped)
		}
	}
	// Dropped facts are declared, never silently swallowed.
	if !strings.Contains(capped, "not shown to keep this small") {
		t.Errorf("truncation not reported:\n%s", capped)
	}
}

func TestContextReturnsEmptyWhenCapCannotFitSearchHeader(t *testing.T) {
	if got := index.BuildContext(loadFixtures(t), index.ContextOptions{Project: "aurora", MaxBytes: 1}); got != "" {
		t.Fatalf("one-byte cap returned %d bytes, want an empty fail-open payload", len(got))
	}
}

func TestContextBudgetPrioritizesCurrentProjectFacts(t *testing.T) {
	all := loadFixtures(t)
	global := all[0]
	for i := 0; i < 100; i++ {
		copy := global
		copy.ID = fmt.Sprintf("global-volume-%03d", i)
		copy.Title = "Global volume fact with enough text to consume a bounded context"
		all = append(all, copy)
	}

	out := index.BuildContext(all, index.ContextOptions{Project: "aurora", MaxBytes: 900})
	if !strings.Contains(out, "Aurora dev server listens on port 8080") {
		t.Fatalf("bounded context dropped current-project facts behind globals:\n%s", out)
	}
	if strings.Index(out, "## Global") > strings.Index(out, "## Project: aurora") {
		t.Fatalf("bounded context changed the documented display order:\n%s", out)
	}
	if !strings.Contains(out, "not shown to keep this small") {
		t.Fatalf("bounded context did not disclose dropped global facts:\n%s", out)
	}
	listed := strings.Count(out, "` — ")
	wantDropped := 105 - listed
	if !strings.Contains(out, fmt.Sprintf("_%d more fact(s) not shown", wantDropped)) {
		t.Fatalf("bounded context reports the wrong dropped count; listed=%d want dropped=%d:\n%s", listed, wantDropped, out)
	}
	if len(out) > 900 {
		t.Fatalf("project-prioritized payload is %d bytes, want at most 900", len(out))
	}
}

func TestEmptyProjectDoesNotConsumeGlobalBudget(t *testing.T) {
	all := loadFixtures(t)
	for i := 0; i < 40; i++ {
		copy := all[0]
		copy.ID = fmt.Sprintf("global-empty-project-%03d", i)
		all = append(all, copy)
	}
	withoutProject := index.BuildContext(all, index.ContextOptions{MaxBytes: 900})
	withEmptyProject := index.BuildContext(all, index.ContextOptions{Project: "missing", MaxBytes: 900})
	if withEmptyProject != withoutProject {
		t.Fatalf("empty project changed bounded global context:\nwithout:\n%s\nwith:\n%s", withoutProject, withEmptyProject)
	}
}

func TestResolveProjectFromGitRemote(t *testing.T) {
	cfg, err := config.Load(filepath.Join(fixtureRoot(t), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	// A repo whose origin basename is aurora-2, which config.toml maps onto
	// the canonical name aurora.
	dir := t.TempDir()
	gitInit(t, dir)
	run(t, dir, "git", "remote", "add", "origin", "git@github.com:someone/aurora-2.git")

	got := project.Resolve(cfg, dir)
	if got.Name != "aurora" {
		t.Errorf("name = %q, want aurora (mapped from aurora-2)", got.Name)
	}
	if got.How != "git-remote" {
		t.Errorf("how = %q, want git-remote", got.How)
	}
	if !got.Mapped {
		t.Errorf("expected the [projects] table to have rewritten the name")
	}
}

func TestResolveProjectFallsBackToBasename(t *testing.T) {
	cfg, err := config.Load(filepath.Join(fixtureRoot(t), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	// Remote-less repo: the basename is the only identity available.
	parent := t.TempDir()
	dir := filepath.Join(parent, "borealis")
	if err := exec.Command("mkdir", "-p", dir).Run(); err != nil {
		t.Fatal(err)
	}
	gitInit(t, dir)

	got := project.Resolve(cfg, dir)
	if got.Name != "borealis" {
		t.Errorf("name = %q, want borealis", got.Name)
	}
	if got.How != "dir-basename" {
		t.Errorf("how = %q, want dir-basename", got.How)
	}

	// A plain directory that is not a git repo at all must still resolve,
	// so the hook is safe to run anywhere.
	plain := filepath.Join(t.TempDir(), "notarepo")
	if err := exec.Command("mkdir", "-p", plain).Run(); err != nil {
		t.Fatal(err)
	}
	if got := project.Resolve(cfg, plain); got.Name != "notarepo" {
		t.Errorf("non-repo dir resolved to %q, want notarepo", got.Name)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init", "-q")
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// A vendored submodule is its own repository with its own remote, so without
// climbing to the superproject a dependency's directory resolves to the
// dependency's name — injecting the wrong project's facts, and filing anything
// written there under a scope that splits the project in two on every visit.
func TestSubmoduleResolvesToItsSuperproject(t *testing.T) {
	cfg, err := config.Load(filepath.Join(fixtureRoot(t), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()
	super := filepath.Join(parent, "aurora")
	vendored := filepath.Join(super, "vendor", "borealis")
	for _, d := range []string{super, vendored} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		gitInit(t, d)
		run(t, d, "git", "config", "user.email", "t@e")
		run(t, d, "git", "config", "user.name", "T")
	}
	run(t, vendored, "git", "commit", "-q", "--allow-empty", "-m", "init")
	// Register it as a real submodule so git reports the superproject.
	run(t, super, "git", "-c", "protocol.file.allow=always", "submodule", "add", "-q", "./vendor/borealis", "vendor/borealis")

	if got := project.Resolve(cfg, super); got.Name != "aurora" {
		t.Errorf("superproject resolved to %q, want aurora", got.Name)
	}
	got := project.Resolve(cfg, vendored)
	if got.Name != "aurora" {
		t.Errorf("submodule resolved to %q, want the superproject aurora — a vendored "+
			"dependency must not become its own project scope", got.Name)
	}
}
