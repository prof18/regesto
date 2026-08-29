package harvest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStableRegularReadRejectsSymlinkReplacement(t *testing.T) {
	dir, outside := t.TempDir(), filepath.Join(t.TempDir(), "outside.md")
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if body, err := readStableRegularFile(path, before); err == nil {
		t.Fatalf("symlink replacement read %q", body)
	}
}
