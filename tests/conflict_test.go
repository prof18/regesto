// Tests for PLAN 2.d's conflict resolution. This is the one place lint
// overwrites a file's content rather than a single field, so the properties that
// matter are: an unresolved conflict is invisible, the loser is archived rather
// than deleted, and anything lint cannot decide is handed to a human.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/lint"
)

var conflictNow = time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)

func conflictStore(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "knowledge", "facts", "global"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func putFact(t *testing.T, root, name, id, modified, body string) string {
	t.Helper()
	path := filepath.Join(root, "knowledge", "facts", "global", name)
	content := "---\nschema_version: 1\nid: " + id + "\ntitle: A claim\ntype: decision\n" +
		"scope: global\nsubject: s\nrelation: r\nstatus: active\nsource: human\n" +
		"created:  2026-07-01T09:00:00Z\nmodified: " + modified + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// An unresolved conflict copy carries the same id as the file it conflicts with.
// If it were loaded, search, the index and the SessionStart hook would each show
// two claims with one identity.
func TestConflictCopiesAreInvisibleUntilResolved(t *testing.T) {
	root := conflictStore(t)
	putFact(t, root, "dec-a.md", "dec-a", "2026-07-01T09:00:00Z", "original")
	putFact(t, root, "dec-a.sync-conflict-20260729-101500-AAA.md", "dec-a", "2026-07-20T09:00:00Z", "other machine")

	all, err := facts.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("loaded %d facts, want 1 — a conflict copy reached the store", len(all))
	}
	if facts.IsConflict(all[0].RelPath) {
		t.Errorf("the conflict copy was loaded instead of the file: %s", all[0].RelPath)
	}
	// It must still be findable, or it could never be resolved.
	conflicts, err := facts.FindConflicts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0].BasePath == "" {
		t.Fatalf("conflict not discovered with its base: %+v", conflicts)
	}
}

func TestNewerConflictCopyWinsInPlace(t *testing.T) {
	root := conflictStore(t)
	base := putFact(t, root, "dec-a.md", "dec-a", "2026-07-01T09:00:00Z", "the old body")
	putFact(t, root, "dec-a.sync-conflict-20260729-101500-AAA.md", "dec-a", "2026-07-20T09:00:00Z", "the newer body")

	conflicts, _ := facts.FindConflicts(root)
	res, err := lint.ResolveConflicts(root, conflicts, true, conflictNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].NeedsHuman {
		t.Fatalf("expected an automatic resolution, got %+v", res)
	}

	// The newer content is now the file, under the original name.
	body, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "the newer body") {
		t.Errorf("newer content did not win in place:\n%s", body)
	}
	// The displaced content is archived, not deleted.
	if res[0].Archived == "" {
		t.Fatal("nothing was archived — the previous content was destroyed")
	}
	archived, err := os.ReadFile(filepath.Join(root, res[0].Archived))
	if err != nil {
		t.Fatalf("archived copy unreadable: %v", err)
	}
	if !strings.Contains(string(archived), "the old body") {
		t.Errorf("the archived copy is not the displaced content:\n%s", archived)
	}
	// And no conflict file remains.
	left, _ := facts.FindConflicts(root)
	if len(left) != 0 {
		t.Errorf("conflict file survived resolution: %+v", left)
	}
}

func TestOlderConflictCopyLosesAndIsArchived(t *testing.T) {
	root := conflictStore(t)
	base := putFact(t, root, "dec-b.md", "dec-b", "2026-07-20T09:00:00Z", "the surviving body")
	putFact(t, root, "dec-b.sync-conflict-20260729-101500-BBB.md", "dec-b", "2026-07-01T09:00:00Z", "the stale body")

	conflicts, _ := facts.FindConflicts(root)
	res, err := lint.ResolveConflicts(root, conflicts, true, conflictNow)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "the surviving body") {
		t.Errorf("the newer file was replaced by a stale copy:\n%s", body)
	}
	if res[0].Archived == "" {
		t.Fatal("the losing copy was deleted rather than archived")
	}
}

// A delete on one machine against an edit on another is not lint's to settle.
func TestDeleteVersusEditNeedsAHuman(t *testing.T) {
	root := conflictStore(t)
	putFact(t, root, "dec-c.sync-conflict-20260729-101500-CCC.md", "dec-c", "2026-07-10T09:00:00Z", "only copy")

	conflicts, _ := facts.FindConflicts(root)
	res, err := lint.ResolveConflicts(root, conflicts, true, conflictNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || !res[0].NeedsHuman {
		t.Fatalf("expected this to be left for a human, got %+v", res)
	}
	// Nothing may have been moved or removed.
	left, _ := facts.FindConflicts(root)
	if len(left) != 1 {
		t.Error("lint acted on a conflict it should have deferred")
	}
}

// A copy that cannot be parsed cannot be compared by `modified`.
func TestUnparseableConflictNeedsAHuman(t *testing.T) {
	root := conflictStore(t)
	putFact(t, root, "dec-d.md", "dec-d", "2026-07-10T09:00:00Z", "fine")
	broken := filepath.Join(root, "knowledge", "facts", "global", "dec-d.sync-conflict-20260729-101500-DDD.md")
	if err := os.WriteFile(broken, []byte("this is not a fact at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	conflicts, _ := facts.FindConflicts(root)
	res, err := lint.ResolveConflicts(root, conflicts, true, conflictNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || !res[0].NeedsHuman {
		t.Fatalf("an unparseable copy should be deferred, got %+v", res)
	}
	if _, err := os.Stat(broken); err != nil {
		t.Error("the unparseable copy was moved despite being deferred")
	}
}

func TestReportOnlyChangesNothing(t *testing.T) {
	root := conflictStore(t)
	putFact(t, root, "dec-a.md", "dec-a", "2026-07-01T09:00:00Z", "old")
	putFact(t, root, "dec-a.sync-conflict-20260729-101500-AAA.md", "dec-a", "2026-07-20T09:00:00Z", "new")

	conflicts, _ := facts.FindConflicts(root)
	res, err := lint.ResolveConflicts(root, conflicts, false, conflictNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Archived != "" {
		t.Errorf("a report-only run moved something: %+v", res)
	}
	left, _ := facts.FindConflicts(root)
	if len(left) != 1 {
		t.Error("a report-only run resolved the conflict")
	}
}

// PLAN 0.2. One project spelled two ways splits its knowledge silently: a
// session in that repo sees only the half matching the name the hook resolved.
func TestScopeAliasesAreCanonicalised(t *testing.T) {
	root := conflictStore(t)
	if err := os.MkdirAll(filepath.Join(root, "knowledge", "facts", "projects", "aurora-2"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nschema_version: 1\nid: fact-a\ntitle: A claim\ntype: fact\n" +
		"scope: project:aurora-2\nsubject: s\nrelation: r\nstatus: active\nsource: codex@studio\n" +
		"created:  2026-07-01T09:00:00Z\nmodified: 2026-07-01T09:00:00Z\n---\n\nThe claim body.\n"
	stray := filepath.Join(root, "knowledge", "facts", "projects", "aurora-2", "fact-a.md")
	if err := os.WriteFile(stray, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("agents = [\"claude\"]\n\n[projects]\n\"aurora-2\" = \"aurora\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	all, err := facts.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}

	fixes, err := lint.CanonicaliseScopes(root, cfg, all, true, conflictNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 1 || fixes[0].Blocked != "" {
		t.Fatalf("expected one clean move, got %+v", fixes)
	}

	moved := filepath.Join(root, "knowledge", "facts", "projects", "aurora", "fact-a.md")
	f, err := facts.ParseFile(moved)
	if err != nil {
		t.Fatalf("fact not at the canonical path: %v", err)
	}
	if f.Scope != "project:aurora" {
		t.Errorf("scope = %q, want project:aurora", f.Scope)
	}
	if !strings.Contains(f.Body, "The claim body.") {
		t.Error("the claim body was altered by a filing correction")
	}
	if _, err := os.Stat(stray); err == nil {
		t.Error("the aliased copy still exists")
	}
}

// A name that merely resembles another must be left alone: beacon and
// beacon-mobile are two real projects.
func TestUnlistedSimilarNamesAreLeftAlone(t *testing.T) {
	root := conflictStore(t)
	dir := filepath.Join(root, "knowledge", "facts", "projects", "beacon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nschema_version: 1\nid: fact-b\ntitle: A claim\ntype: fact\n" +
		"scope: project:beacon\nsubject: s\nrelation: r\nstatus: active\nsource: codex@studio\n" +
		"created:  2026-07-01T09:00:00Z\nmodified: 2026-07-01T09:00:00Z\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, "fact-b.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("agents = [\"claude\"]\n\n[projects]\n\"aurora-2\" = \"aurora\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(cfgPath)
	all, _ := facts.LoadAll(root)

	fixes, err := lint.CanonicaliseScopes(root, cfg, all, true, conflictNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 0 {
		t.Errorf("an unlisted project name was merged by resemblance: %+v", fixes)
	}
}

// The conflict naming is the one place regesto knows anything about a specific
// sync client, so it is configurable. A pattern rather than a literal marker,
// because clients differ in shape: Syncthing appends before the extension,
// others bracket their insertion mid-name — and taking "the text before a
// marker" only ever handles the first.
func TestConflictPatternFollowsTheSyncClient(t *testing.T) {
	t.Cleanup(func() { _ = facts.SetConflictPattern(facts.DefaultConflictPattern) })

	syncthing := "dec-a.sync-conflict-20260729-101500-ABCDEF.md"
	dropbox := "dec-a (someone's conflicted copy 2026-07-30).md"

	// The default knows Syncthing and nothing else.
	if !facts.IsConflict(syncthing) {
		t.Error("default pattern failed to recognise a Syncthing conflict copy")
	}
	if got := facts.BaseName(syncthing); got != "dec-a.md" {
		t.Errorf("BaseName = %q, want dec-a.md", got)
	}
	if facts.IsConflict(dropbox) {
		t.Error("default pattern claimed another client's naming, which it cannot resolve")
	}

	if err := facts.SetConflictPattern(` \(.*conflicted copy.*\)`); err != nil {
		t.Fatal(err)
	}
	if !facts.IsConflict(dropbox) {
		t.Error("configured pattern failed to recognise the client's conflict copy")
	}
	if got := facts.BaseName(dropbox); got != "dec-a.md" {
		t.Errorf("BaseName = %q, want dec-a.md — the insertion must be cut out, not truncated at", got)
	}
	// An ordinary fact must never look like a conflict, whatever the pattern.
	if facts.IsConflict("dec-a.md") {
		t.Error("a plain fact filename was taken for a conflict copy")
	}
}

// A bad pattern has to fail loudly at startup. Silently keeping the default
// would mean conflict copies loading as real facts, two claims sharing one id.
func TestInvalidConflictPatternIsRejected(t *testing.T) {
	t.Cleanup(func() { _ = facts.SetConflictPattern(facts.DefaultConflictPattern) })
	if err := facts.SetConflictPattern(`([unclosed`); err == nil {
		t.Error("an invalid regular expression was accepted")
	}
	// The default must survive a rejected one.
	if !facts.IsConflict("dec-a.sync-conflict-20260729-101500-ABCDEF.md") {
		t.Error("a rejected pattern damaged the working default")
	}
}
