// Tests for PLAN 2.a. Harvest must need no cooperation from the agent, must
// never write to a vendor directory, and must not lose a capture if writing the
// inbox fails.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/harvest"
)

// harvestInstance builds a KB plus a fake agent memory directory.
func harvestInstance(t *testing.T) (cfg *config.Config, memDir string) {
	t.Helper()
	return harvestInstanceWith(t, "")
}

func harvestInstanceWith(t *testing.T, extra string) (cfg *config.Config, memDir string) {
	t.Helper()
	t.Setenv("REGESTO_MACHINE", "testbox")
	root := t.TempDir()
	memDir = filepath.Join(t.TempDir(), "agentmem", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "integrations = [\"claude\"]\n\n[integrations.claude]\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \"" + memDir + "\"\n" + extra
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, memDir
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func captured(results []harvest.Result) []string {
	var out []string
	for _, r := range results {
		out = append(out, r.Captured...)
	}
	return out
}

func inboxFiles(t *testing.T, cfg *config.Config) []string {
	t.Helper()
	var out []string
	root := filepath.Join(cfg.KBRoot, "inbox")
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		out = append(out, filepath.Base(path))
		return nil
	})
	return out
}

// The first pass must record a baseline rather than dumping an agent's whole
// existing store into the inbox — that content is already covered by 0.c.
func TestFirstRunRecordsBaselineWithoutCapturing(t *testing.T) {
	cfg, mem := harvestInstance(t)
	write(t, mem, "note.md", "an existing note")

	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 0 {
		t.Errorf("first run captured %v, want none", got)
	}
	if files := inboxFiles(t, cfg); len(files) != 0 {
		t.Errorf("first run wrote to the inbox: %v", files)
	}
}

func TestSubsequentWriteIsCaptured(t *testing.T) {
	cfg, mem := harvestInstance(t)
	write(t, mem, "note.md", "an existing note")
	if _, err := harvest.Run(cfg, false); err != nil { // baseline
		t.Fatal(err)
	}

	write(t, mem, "new-fact.md", "the agent wrote this by itself")
	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	got := captured(results)
	if len(got) != 1 || !strings.HasSuffix(got[0], "new-fact.md") {
		t.Fatalf("captured %v, want just new-fact.md", got)
	}
	found := false
	for _, f := range inboxFiles(t, cfg) {
		if strings.HasSuffix(f, "new-fact.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("capture not written to the inbox: %v", inboxFiles(t, cfg))
	}

	// A third pass with nothing changed must be silent.
	results, err = harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 0 {
		t.Errorf("unchanged tree captured %v, want none", got)
	}
}

// A change to an already-seen file is captured as a diff, not a fresh copy. The
// inbox is synced and committed; the baseline it diffs against lives in .state/,
// which is neither. That keeps a large, rarely-changing store from paying its
// full size into the repository every time one line moves.
func TestChangeToSeenFileIsCapturedAsADiff(t *testing.T) {
	cfg, mem := harvestInstance(t)
	bulk := strings.Repeat("a line of existing content\n", 4000) // ~108KB
	write(t, mem, "big.md", bulk)
	if _, err := harvest.Run(cfg, false); err != nil { // baseline, and seeds the blob
		t.Fatal(err)
	}
	write(t, mem, "big.md", bulk+"\nthe one new line the agent added\n")

	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	var bytesWritten int64
	for _, r := range results {
		bytesWritten += r.CapturedBytes
	}
	if bytesWritten == 0 {
		t.Fatal("nothing captured")
	}
	if bytesWritten > int64(len(bulk))/10 {
		t.Errorf("capture was %d bytes for a one-line change to a %d byte file — not a diff",
			bytesWritten, len(bulk))
	}

	var names []string
	names = append(names, inboxFiles(t, cfg)...)
	foundDiff := false
	for _, n := range names {
		if strings.HasSuffix(n, ".diff") {
			foundDiff = true
		}
	}
	if !foundDiff {
		t.Errorf("expected a .diff capture, got %v", names)
	}

	// The new line must actually be in there — a small capture that lost the
	// content would be worse than a large one.
	var body []byte
	_ = filepath.Walk(filepath.Join(cfg.KBRoot, "inbox"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".diff") {
			body, _ = os.ReadFile(p)
		}
		return nil
	})
	if !strings.Contains(string(body), "the one new line the agent added") {
		t.Errorf("diff does not contain the added line:\n%s", body)
	}
}

// An edit to a file already seen is a new capture: the agent revised its note.
func TestEditedFileIsRecaptured(t *testing.T) {
	cfg, mem := harvestInstance(t)
	write(t, mem, "note.md", "version one")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, mem, "note.md", "version two, revised")

	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 1 {
		t.Fatalf("edited file captured %v, want one entry", got)
	}
}

// Nothing in harvest may modify the agent's own files — that is what keeps
// "never write a vendor file before harvesting it" easy to hold.
func TestHarvestNeverWritesToVendorDirectory(t *testing.T) {
	cfg, mem := harvestInstance(t)
	write(t, mem, "note.md", "original content")
	before, err := os.ReadFile(filepath.Join(mem, "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	beforeList, err := os.ReadDir(mem)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, mem, "second.md", "more")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(filepath.Join(mem, "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("harvest modified an agent's memory file")
	}
	afterList, err := os.ReadDir(mem)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterList) != len(beforeList)+1 { // only our own test write
		t.Errorf("vendor directory gained unexpected entries: %d -> %d", len(beforeList), len(afterList))
	}
}

// A dry run must be observable but inert.
func TestDryRunWritesNothing(t *testing.T) {
	cfg, mem := harvestInstance(t)
	write(t, mem, "note.md", "one")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, mem, "two.md", "two")

	results, err := harvest.Run(cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 1 {
		t.Errorf("dry run should still report what it would capture, got %v", got)
	}
	if files := inboxFiles(t, cfg); len(files) != 0 {
		t.Errorf("dry run wrote to the inbox: %v", files)
	}
	// And having written nothing, the next real run must still capture it.
	results, err = harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 1 {
		t.Errorf("dry run consumed the capture; real run got %v", got)
	}
}

// The size guard exists for a pathological file, not as a content filter — so
// the default is generous and the limit has to be set deliberately to bite.
func TestOversizeFilesAreSkippedNotCaptured(t *testing.T) {
	cfg, mem := harvestInstanceWith(t, "[harvest]\nmax_capture_bytes = \"65536\"\n")
	write(t, mem, "small.md", "a note")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, mem, "huge.md", strings.Repeat("x", 200*1024))

	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 0 {
		t.Errorf("oversize file was captured: %v", got)
	}
	var skipped []string
	for _, r := range results {
		skipped = append(skipped, r.Skipped...)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "huge.md") {
		t.Errorf("oversize file should be reported as skipped, got %v", skipped)
	}
}

func TestGenericMarkdownMemoryIsHarvestedWithoutEngineChanges(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	root, mem := t.TempDir(), filepath.Join(t.TempDir(), "generic-memory")
	if err := os.MkdirAll(mem, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \"" + mem + "\"\n"
	cfg := loadHarvestConfig(t, root, body)
	write(t, mem, "existing.md", "baseline")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, mem, "new.md", "captured")
	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 1 || !strings.HasSuffix(got[0], "new.md") {
		t.Fatalf("generic captures = %v", got)
	}
	if len(results) != 1 || results[0].Kind != "markdown-glob-v1" || results[0].Location != mem {
		t.Fatalf("generic source result = %#v", results)
	}
}

func TestNoneAndUnsupportedMemoryKindsAreExplicit(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "none", body: "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\n", want: "kind none"},
		{name: "unsupported", body: "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"sqlite-v1\"\n", want: "unsupported memory kind"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := loadHarvestConfig(t, t.TempDir(), test.body)
			results, err := harvest.Run(cfg, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 || !strings.Contains(results[0].Note, test.want) {
				t.Fatalf("memory status = %#v", results)
			}
			if files := inboxFiles(t, cfg); len(files) != 0 {
				t.Fatalf("unsupported source wrote inbox files: %v", files)
			}
		})
	}
}

func TestMissingAndMalformedMarkdownSourcesAreObservable(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	root := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing", "*")
	cfg := loadHarvestConfig(t, root, "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \""+missing+"\"\n")
	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !strings.Contains(results[0].Note, "source not present") {
		t.Fatalf("missing source result = %#v", results)
	}

	badGlob := filepath.Join(t.TempDir(), "[")
	cfg = loadHarvestConfig(t, root, "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \""+badGlob+"\"\n")
	if _, err := harvest.Run(cfg, false); err == nil || !strings.Contains(err.Error(), "bad markdown-glob-v1") {
		t.Fatalf("malformed glob error = %v", err)
	}
}

func TestMultipleMarkdownSourcesKeepIndependentStableBaselines(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	one, two := filepath.Join(home, ".multi", "one"), filepath.Join(home, ".multi", "two")
	for _, dir := range []string{one, two, filepath.Join(root, "adapters", "profiles")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	profilePath := filepath.Join(root, "adapters", "profiles", "multi.json")
	profile := func(first, second string) []byte {
		return []byte(`{"schema_version":1,"id":"multi","display_name":"Multi","skills":{"targets":[],"variant":"portable"},"instructions":{"targets":[]},"hooks":[{"protocol":"none","registrar":"none"}],"memory":[{"kind":"markdown-glob-v1","location":"` + first + `"},{"kind":"markdown-glob-v1","location":"` + second + `"}],"default_trust":"quarantine"}`)
	}
	if err := os.WriteFile(profilePath, profile("~/.multi/one", "~/.multi/two"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := loadHarvestConfig(t, root, "integrations = [\"multi\"]\n")
	write(t, one, "one.md", "one baseline")
	write(t, two, "two.md", "two baseline")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, one, "one-new.md", "one capture")
	write(t, two, "two-new.md", "two capture")
	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 2 {
		t.Fatalf("multiple-source captures = %v", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".state", "testbox", "multi.json")); err != nil {
		t.Fatalf("snapshot path missing: %v", err)
	}

	// Source order is presentation, not identity. Reordering a profile must not
	// baseline or recapture either store.
	if err := os.WriteFile(profilePath, profile("~/.multi/two", "~/.multi/one"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg = loadHarvestConfig(t, root, "integrations = [\"multi\"]\n")
	results, err = harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 0 {
		t.Fatalf("reordered sources recaptured %v", got)
	}
}

func TestHarvestSkipsSymlinkedMarkdownEntries(t *testing.T) {
	cfg, mem := harvestInstance(t)
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(foreign, []byte("do not capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, filepath.Join(mem, "alias.md")); err != nil {
		t.Fatal(err)
	}
	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 0 {
		t.Fatalf("symlink capture = %v", got)
	}
	if len(results) != 1 || len(results[0].Skipped) != 1 || !strings.Contains(results[0].Skipped[0], "symlinked") {
		t.Fatalf("symlink skip result = %#v", results)
	}
	if got, err := os.ReadFile(foreign); err != nil || string(got) != "do not capture" {
		t.Fatalf("foreign file changed: %q, %v", got, err)
	}
}

func TestHarvestFollowsDeclaredSymlinkedMemoryDirectory(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	root, parent, target := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "memory-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	cfg := loadHarvestConfig(t, root, "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \""+link+"\"\n")
	write(t, target, "existing.md", "baseline")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, target, "new.md", "capture through declared symlink")
	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 1 || !strings.HasSuffix(got[0], "new.md") {
		t.Fatalf("symlinked-root captures = %v", got)
	}
}

func TestOverlappingSourcesRemainStableAcrossReorderAndReaddition(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	memory := filepath.Join(home, ".overlap", "memory")
	child := filepath.Join(memory, "child")
	profiles := filepath.Join(root, "adapters", "profiles")
	for _, dir := range []string{child, profiles} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	profilePath := filepath.Join(profiles, "overlap.json")
	profile := func(locations ...string) []byte {
		var memoryJSON []string
		for _, location := range locations {
			memoryJSON = append(memoryJSON, `{"kind":"markdown-glob-v1","location":"`+location+`"}`)
		}
		return []byte(`{"schema_version":1,"id":"overlap","display_name":"Overlap","skills":{"targets":[],"variant":"portable"},"instructions":{"targets":[]},"hooks":[{"protocol":"none","registrar":"none"}],"memory":[` + strings.Join(memoryJSON, ",") + `],"default_trust":"quarantine"}`)
	}
	writeProfile := func(locations ...string) {
		t.Helper()
		if err := os.WriteFile(profilePath, profile(locations...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeProfile("~/.overlap/memory", "~/.overlap/memory/child")
	write(t, child, "note.md", "baseline")
	cfg := loadHarvestConfig(t, root, "integrations = [\"overlap\"]\n")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, child, "new.md", "version one")
	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 1 {
		t.Fatalf("overlap captured %v, want one publication", got)
	}

	writeProfile("~/.overlap/memory/child", "~/.overlap/memory")
	cfg = loadHarvestConfig(t, root, "integrations = [\"overlap\"]\n")
	if results, err = harvest.Run(cfg, false); err != nil || len(captured(results)) != 0 {
		t.Fatalf("reordered overlap results = %#v, %v", results, err)
	}

	writeProfile("~/.overlap/memory")
	cfg = loadHarvestConfig(t, root, "integrations = [\"overlap\"]\n")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, child, "new.md", "version two")
	if results, err = harvest.Run(cfg, false); err != nil || len(captured(results)) != 1 {
		t.Fatalf("single active overlap source results = %#v, %v", results, err)
	}

	writeProfile("~/.overlap/memory/child", "~/.overlap/memory")
	cfg = loadHarvestConfig(t, root, "integrations = [\"overlap\"]\n")
	if results, err = harvest.Run(cfg, false); err != nil || len(captured(results)) != 0 {
		t.Fatalf("re-added overlap source duplicated capture = %#v, %v", results, err)
	}
}

func TestHarvestRejectsUnsafeStateNamespaceComponents(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	root := t.TempDir()
	cfg := loadHarvestConfig(t, root, "integrations = [\"../escape\"]\n")
	if _, err := harvest.Run(cfg, false); err == nil || !strings.Contains(err.Error(), "safe inbox/state path component") {
		t.Fatalf("unsafe integration error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape@testbox")); !os.IsNotExist(err) {
		t.Fatalf("unsafe integration created an escaped path: %v", err)
	}

	t.Setenv("REGESTO_MACHINE", "../escape")
	cfg = loadHarvestConfig(t, root, "integrations = [\"claude\"]\n")
	if _, err := harvest.Run(cfg, false); err == nil || !strings.Contains(err.Error(), "machine name") {
		t.Fatalf("unsafe machine error = %v", err)
	}
}

func TestHarvestRejectsKnowledgeBaseAndMemoryOverlapBeforeWriting(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	parent := t.TempDir()
	root := filepath.Join(parent, "kb")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := loadHarvestConfig(t, root, "integrations = [\"claude\"]\n[integrations.claude]\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \""+parent+"\"\n")
	if _, err := harvest.Run(cfg, false); err == nil || !strings.Contains(err.Error(), "overlaps knowledge-base root") {
		t.Fatalf("overlap error = %v", err)
	}
	for _, path := range []string{filepath.Join(root, ".state", "testbox"), filepath.Join(root, "inbox")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("overlap preflight wrote %s: %v", path, err)
		}
	}
}

func TestHarvestRejectsSymlinkedOutputRootsWithoutTouchingVendor(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	for _, output := range []string{".state", "inbox"} {
		t.Run(output, func(t *testing.T) {
			root, memory, vendor := t.TempDir(), filepath.Join(t.TempDir(), "memory"), t.TempDir()
			if err := os.MkdirAll(memory, 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, memory, "note.md", "vendor memory")
			if err := os.Symlink(vendor, filepath.Join(root, output)); err != nil {
				t.Fatal(err)
			}
			cfg := loadHarvestConfig(t, root, "integrations = [\"claude\"]\n[integrations.claude]\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \""+memory+"\"\n")
			if _, err := harvest.Run(cfg, false); err == nil || !strings.Contains(err.Error(), "output root") {
				t.Fatalf("symlinked %s error = %v", output, err)
			}
			entries, err := os.ReadDir(vendor)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("symlinked %s mutated vendor target: %v", output, entries)
			}
		})
	}
}

func TestNestedParentSymlinkAliasesPublishOnce(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "testbox")
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	realParent := filepath.Join(home, ".alias-real")
	memory := filepath.Join(realParent, "memory")
	if err := os.MkdirAll(memory, 0o755); err != nil {
		t.Fatal(err)
	}
	aliasParent := filepath.Join(home, ".alias")
	if err := os.Symlink(realParent, aliasParent); err != nil {
		t.Fatal(err)
	}
	profiles := filepath.Join(root, "adapters", "profiles")
	if err := os.MkdirAll(profiles, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version":1,"id":"alias","display_name":"Alias","skills":{"targets":[],"variant":"portable"},"instructions":{"targets":[]},"hooks":[{"protocol":"none","registrar":"none"}],"memory":[{"kind":"markdown-glob-v1","location":"~/.alias-real/memory"},{"kind":"markdown-glob-v1","location":"~/.alias/memory"}],"default_trust":"quarantine"}`
	if err := os.WriteFile(filepath.Join(profiles, "alias.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := loadHarvestConfig(t, root, "integrations = [\"alias\"]\n")
	write(t, memory, "existing.md", "baseline")
	if _, err := harvest.Run(cfg, false); err != nil {
		t.Fatal(err)
	}
	write(t, memory, "new.md", "one physical change")
	results, err := harvest.Run(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := captured(results); len(got) != 1 {
		t.Fatalf("aliased sources published %v", got)
	}
}

func loadHarvestConfig(t *testing.T, root, body string) *config.Config {
	t.Helper()
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
