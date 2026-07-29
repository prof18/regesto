// Tests for PLAN 2.a. Harvest must need no cooperation from the agent, must
// never write to a vendor directory, and must not lose a capture if writing the
// inbox fails.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regesto/internal/config"
	"regesto/internal/harvest"
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
	body := "agents = [\"claude\"]\n\n[memory_dirs]\nclaude = \"" + memDir + "\"\n" + extra
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
