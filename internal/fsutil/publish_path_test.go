package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishReplacePathReplacesContentAndKeepsMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	dest := filepath.Join(dir, "fact.md")
	if err := os.WriteFile(dest, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, ".regesto-tmp")
	if err := os.WriteFile(tmp, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tmp, 0o640); err != nil {
		t.Fatal(err)
	}

	if err := PublishReplacePath(tmp, dest); err != nil {
		t.Fatalf("PublishReplacePath: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("dest content = %q, want %q", got, "new\n")
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("dest mode = %o, want 640", info.Mode().Perm())
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temp should be consumed, stat err = %v", err)
	}
}

func TestPublishReplacePathCreatesWhenDestAbsent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tmp := filepath.Join(dir, ".regesto-tmp")
	if err := os.WriteFile(tmp, []byte("fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "fact.md")
	if err := PublishReplacePath(tmp, dest); err != nil {
		t.Fatalf("PublishReplacePath: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fresh\n" {
		t.Fatalf("dest content = %q", got)
	}
}

// The fallback's exclusive re-create has to land in the destination's own
// directory, so a cross-directory publish is refused rather than half-honoured.
func TestPublishReplacePathRefusesCrossDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	other := t.TempDir()

	tmp := filepath.Join(other, ".regesto-tmp")
	if err := os.WriteFile(tmp, []byte("elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "fact.md")
	if err := PublishReplacePath(tmp, dest); err == nil {
		t.Fatal("expected a cross-directory publish to be refused")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("refused publish should not have created dest, stat err = %v", err)
	}
}
