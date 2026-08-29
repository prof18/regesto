package tests

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	writeop "github.com/prof18/regesto/internal/write"
)

func validWriteInput(id string) writeop.Input {
	return writeop.Input{
		ID: id, Title: `A "quoted": write`, Type: "decision", Scope: "global",
		Subject: "writer", Relation: "contract", Topics: []string{"regesto", "cli"},
		Body: "The validated writer owns persistence.", Why: "authority fields must not be forgeable",
	}
}

func TestWriteStampsAuthorityAndCreatesCanonicalFact(t *testing.T) {
	cfg := normalizeInstance(t)
	now := time.Date(2026, 8, 29, 12, 34, 56, 0, time.UTC)
	in := validWriteInput("dec-validated-write")

	result, err := writeop.Create(cfg, in, writeop.Authority{Source: "codex@testbox", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "knowledge/facts/global/dec-validated-write.md" || result.SchemaVersion != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Actions == nil || result.Reviews == nil {
		t.Fatalf("empty result collections must encode as []: %+v", result)
	}
	f, err := facts.ParseFile(filepath.Join(cfg.KBRoot, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if f.Source != "codex@testbox" || f.SchemaVersion != "1" {
		t.Errorf("authority not stamped: %+v", f)
	}
	if f.Created != now.Format(time.RFC3339) || f.Modified != f.Created {
		t.Errorf("timestamps not stamped from authority: %q / %q", f.Created, f.Modified)
	}
	if f.Title != in.Title || !strings.Contains(f.Body, "**Why:** "+in.Why) {
		t.Errorf("semantic content did not round-trip: %+v", f)
	}
}

func TestWriteRejectsNonCanonicalSource(t *testing.T) {
	cfg := normalizeInstance(t)
	if _, err := writeop.Create(cfg, validWriteInput("dec-bad-source"), writeop.Authority{Source: "bogus"}); err == nil || !strings.Contains(err.Error(), "<integration>@<machine>") {
		t.Fatalf("noncanonical source was not rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "dec-bad-source.md")); !os.IsNotExist(err) {
		t.Fatalf("invalid-source write reached disk: %v", err)
	}
}

func TestWriteProjectPathAndTraversalValidation(t *testing.T) {
	cfg := normalizeInstance(t)
	in := validWriteInput("dec-project-write")
	in.Scope = "project:aurora"
	result, err := writeop.Create(cfg, in, writeop.Authority{Source: "codex@testbox"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "knowledge/facts/projects/aurora/dec-project-write.md" {
		t.Fatalf("project path = %q", result.Path)
	}

	bad := validWriteInput("dec-path-escape")
	bad.Scope = "project:../../outside"
	if _, err := writeop.Create(cfg, bad, writeop.Authority{Source: "codex@testbox"}); err == nil {
		t.Fatal("traversal-like project scope was accepted")
	}
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "outside", "dec-path-escape.md")); !os.IsNotExist(err) {
		t.Fatalf("invalid write escaped the fact tree: %v", err)
	}
}

func TestWriteRejectsProjectDirectorySymlinkOutsideKB(t *testing.T) {
	cfg := normalizeInstance(t)
	outside := t.TempDir()
	projects := filepath.Join(cfg.KBRoot, "knowledge", "facts", "projects")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(projects, "escaped")); err != nil {
		t.Fatal(err)
	}
	in := validWriteInput("dec-symlink-escape")
	in.Scope = "project:escaped"
	if _, err := writeop.Create(cfg, in, writeop.Authority{Source: "codex@testbox"}); err == nil {
		t.Fatalf("outside symlink was not rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, in.ID+".md")); !os.IsNotExist(err) {
		t.Fatalf("symlinked write escaped the KB: %v", err)
	}
}

func TestWriteRejectsSymlinkedCanonicalFact(t *testing.T) {
	cfg := normalizeInstance(t)
	external := filepath.Join(t.TempDir(), "external.md")
	raw := "---\nschema_version: 1\nid: fact-external\ntitle: External\ntype: fact\nscope: global\nsubject: writer\nrelation: symlink\ntopics: []\nstatus: active\nsource: human\ncreated: 2026-08-29T00:00:00Z\nmodified: 2026-08-29T00:00:00Z\n---\n\nExternal claim.\n"
	if err := os.WriteFile(external, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "fact-external.md")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := writeop.Create(cfg, validWriteInput("dec-after-symlink"), writeop.Authority{Source: "codex@testbox"}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked canonical fact was not rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "dec-after-symlink.md")); !os.IsNotExist(err) {
		t.Fatalf("write proceeded after unsafe fact: %v", err)
	}
}

func TestWriteRejectsSymlinkedLockRoot(t *testing.T) {
	cfg := normalizeInstance(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(cfg.KBRoot, ".state")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := writeop.Create(cfg, validWriteInput("dec-lock-symlink"), writeop.Authority{Source: "codex@testbox"}); err == nil || !strings.Contains(err.Error(), "write lock component") {
		t.Fatalf("symlinked lock root was not rejected: %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("lock write escaped KB: %v", entries)
	}
}

func TestWriteStoreLimitIsAtomicAcrossDifferentIDs(t *testing.T) {
	cfg := normalizeInstance(t)
	inputs := []writeop.Input{validWriteInput("dec-capacity-a"), validWriteInput("dec-capacity-b")}
	for i := range inputs {
		inputs[i].Subject = inputs[i].ID
		inputs[i].Body = strings.Repeat("x", 1200)
	}
	start := make(chan struct{})
	errs := make(chan error, len(inputs))
	for _, input := range inputs {
		input := input
		go func() {
			<-start
			_, err := writeop.Create(cfg, input, writeop.Authority{
				Source: "codex@testbox", MaxFactBytes: 2000, MaxStoreBytes: 2000, MaxFactCount: 10,
			})
			errs <- err
		}()
	}
	close(start)
	var succeeded, limited int
	for range inputs {
		if err := <-errs; err == nil {
			succeeded++
		} else if strings.Contains(err.Error(), "canonical facts exceed 2000 bytes") {
			limited++
		} else {
			t.Fatalf("unexpected concurrent write error: %v", err)
		}
	}
	if succeeded != 1 || limited != 1 {
		t.Fatalf("concurrent limited writes: succeeded=%d limited=%d", succeeded, limited)
	}
	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("published %d facts, want exactly one", len(all))
	}
}

func TestWriteDuplicateIDNeverOverwrites(t *testing.T) {
	cfg := normalizeInstance(t)
	in := validWriteInput("dec-unique-write")
	first, err := writeop.Create(cfg, in, writeop.Authority{Source: "codex@testbox"})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cfg.KBRoot, filepath.FromSlash(first.Path))
	original, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	in.Body = "This must never replace the first claim."
	if _, err := writeop.Create(cfg, in, writeop.Authority{Source: "claude@testbox"}); err == nil {
		t.Fatal("duplicate id was accepted")
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("duplicate write changed the incumbent file")
	}
}

func TestWriteConcurrentDuplicatePublishesExactlyOnce(t *testing.T) {
	cfg := normalizeInstance(t)
	inputs := []writeop.Input{validWriteInput("dec-racing-write"), validWriteInput("dec-racing-write")}
	// Different target paths exercise the global-ID lock, not merely the
	// no-replace publication on one path.
	inputs[1].Scope = "project:aurora"
	otherMachine := *cfg
	otherMachine.Machine = "otherbox"
	configs := []*config.Config{cfg, &otherMachine}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(runCfg *config.Config, in writeop.Input) {
			defer wg.Done()
			<-start
			_, err := writeop.Create(runCfg, in, writeop.Authority{Source: "codex@testbox"})
			errs <- err
		}(configs[i], inputs[i])
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent duplicate successes = %d, want 1", successes)
	}
	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "dec-racing-write" {
		t.Fatalf("published facts are partial or globally duplicated: %+v", all)
	}
}

func TestWriteAgentContestWithHumanIsProposedForReview(t *testing.T) {
	cfg := normalizeInstance(t)
	human := validWriteInput("dec-human-target")
	human.Subject, human.Relation = "deploy", "target"
	if _, err := writeop.Create(cfg, human, writeop.Authority{Source: "human", Now: time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	agent := validWriteInput("dec-agent-target")
	agent.Subject, agent.Relation = "deploy", "target"
	result, err := writeop.Create(cfg, agent, writeop.Authority{Source: "codex@testbox", Now: time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !result.PendingReconciliation || len(result.Reviews) == 0 {
		t.Fatalf("human contest was not reported: %+v", result)
	}
	written, err := facts.ParseFile(filepath.Join(cfg.KBRoot, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if written.Status != facts.StatusProposed || written.Supersedes != human.ID {
		t.Errorf("agent contest did not land proposed with intent: %+v", written)
	}
	incumbent, err := facts.ParseFile(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", human.ID+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if incumbent.Status != facts.StatusActive {
		t.Errorf("human incumbent was mutated during write: %+v", incumbent)
	}
}

func TestWriteAgentCannotDeclareHumanClaimSuperseded(t *testing.T) {
	cfg := normalizeInstance(t)
	human := validWriteInput("dec-human-incumbent")
	human.Subject, human.Relation = "release", "channel"
	if _, err := writeop.Create(cfg, human, writeop.Authority{Source: "human", Now: time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	agent := validWriteInput("dec-agent-challenger")
	agent.Subject, agent.Relation = "release", "channel"
	agent.Status = facts.StatusActive
	agent.Supersedes = human.ID
	result, err := writeop.Create(cfg, agent, writeop.Authority{Source: "claude@testbox", Now: time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	written, err := facts.ParseFile(filepath.Join(cfg.KBRoot, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if written.Status != facts.StatusProposed || written.Supersedes != human.ID || !result.PendingReconciliation {
		t.Fatalf("agent was allowed to pre-accept a human supersession: fact=%+v result=%+v", written, result)
	}
}

func TestWriteRejectsSupersedingDifferentClaimIdentity(t *testing.T) {
	cfg := normalizeInstance(t)
	old := validWriteInput("dec-original-identity")
	old.Subject, old.Relation = "deploy", "target"
	if _, err := writeop.Create(cfg, old, writeop.Authority{Source: "codex@testbox"}); err != nil {
		t.Fatal(err)
	}
	challenger := validWriteInput("dec-unrelated-supersession")
	challenger.Subject, challenger.Relation = "release", "channel"
	challenger.Supersedes = old.ID
	if _, err := writeop.Create(cfg, challenger, writeop.Authority{Source: "codex@testbox"}); err == nil || !strings.Contains(err.Error(), "has identity") {
		t.Fatalf("unrelated supersession was not rejected: %v", err)
	}
	incumbent, err := facts.ParseFile(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", old.ID+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if incumbent.Status != facts.StatusActive {
		t.Fatalf("unrelated incumbent was mutated: %+v", incumbent)
	}
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", challenger.ID+".md")); !os.IsNotExist(err) {
		t.Fatalf("invalid challenger reached disk: %v", err)
	}
}
