package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMovePathMovesBetweenDirectories(t *testing.T) {
	t.Parallel()
	from := t.TempDir()
	to := t.TempDir()

	src := filepath.Join(from, "dec-a.md")
	if err := os.WriteFile(src, []byte("claim\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(to, "dec-a.md")

	if err := MovePath(src, dest); err != nil {
		t.Fatalf("MovePath: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "claim\n" {
		t.Fatalf("dest content = %q", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be gone, stat err = %v", err)
	}
}

// The copy tier is what runs where rename is rejected, and it is exercised
// directly because no local filesystem rejects rename.
func TestMoveByCopyKeepsContentAndModeAndConsumesSource(t *testing.T) {
	t.Parallel()
	from := t.TempDir()
	to := t.TempDir()

	src := filepath.Join(from, "dec-a.md")
	if err := os.WriteFile(src, []byte("claim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0o640); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(to, "dec-a.md")

	if err := moveByCopy(src, dest); err != nil {
		t.Fatalf("moveByCopy: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "claim\n" {
		t.Fatalf("dest content = %q", got)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("dest mode = %o, want 640", info.Mode().Perm())
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be consumed, stat err = %v", err)
	}
	// No temporary left in the destination directory.
	entries, err := os.ReadDir(to)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "dec-a.md" {
		t.Fatalf("destination directory holds %v, want only dec-a.md", entries)
	}
}

// Without a working rename the fallback claims the destination with O_EXCL, so
// a name another writer already holds is refused rather than overwritten.
func TestMoveByCopyRefusesExistingDestination(t *testing.T) {
	t.Parallel()
	from := t.TempDir()
	to := t.TempDir()

	src := filepath.Join(from, "dec-a.md")
	if err := os.WriteFile(src, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(to, "dec-a.md")
	if err := os.WriteFile(dest, []byte("theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := moveByCopy(src, dest); err == nil {
		t.Fatal("expected the occupied destination to be refused")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "theirs\n" {
		t.Fatalf("destination was overwritten: %q", got)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should survive a refused move: %v", err)
	}
}

// A move that publishes the copy but cannot remove the source rolls the
// destination back, so one fact is never left living under two paths — which
// for a scope move would be one id in two project directories.
func TestMoveByCopyRollsBackWhenSourceSurvives(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions, so the removal cannot be made to fail")
	}
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not block removal on Windows")
	}
	from := t.TempDir()
	to := t.TempDir()

	src := filepath.Join(from, "dec-a.md")
	if err := os.WriteFile(src, []byte("claim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A read-only parent directory is what makes removing the source fail.
	if err := os.Chmod(from, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(from, 0o700) })

	dest := filepath.Join(to, "dec-a.md")
	if err := moveByCopy(src, dest); err == nil {
		t.Fatal("expected the move to fail when the source cannot be removed")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("destination should have been rolled back, stat err = %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should still be there: %v", err)
	}
}

// CopyPath archives without disturbing the original, and claims the
// destination exclusively.
func TestCopyPathKeepsSourceAndRefusesOccupiedName(t *testing.T) {
	t.Parallel()
	from := t.TempDir()
	to := t.TempDir()

	src := filepath.Join(from, "dec-a.md")
	if err := os.WriteFile(src, []byte("claim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(to, "dec-a.md.superseded")
	if err := CopyPath(src, dest); err != nil {
		t.Fatalf("CopyPath: %v", err)
	}
	for _, p := range []string{src, dest} {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "claim\n" {
			t.Fatalf("%s content = %q", p, got)
		}
	}
	if err := CopyPath(src, dest); err == nil {
		t.Fatal("expected a second copy onto the same name to be refused")
	}
}

// The executable fallback must not write into the old file: a process running
// it, or a shell part-way through reading it, keeps the old inode and its old
// content, and the name gets a new file.
func TestReplaceByNewFileNeverRewritesAnExecutableInPlace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := os.WriteFile(filepath.Join(dir, "regesto-search"), []byte("#!/bin/sh\necho old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tmp"), []byte("#!/bin/sh\necho new\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	running, err := os.Open(filepath.Join(dir, "regesto-search"))
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	before, err := running.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if err := replaceByNewFile(root, ".tmp", "regesto-search"); err != nil {
		t.Fatalf("replaceByNewFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "regesto-search"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "#!/bin/sh\necho new\n" {
		t.Fatalf("published content = %q", got)
	}
	after, err := os.Stat(filepath.Join(dir, "regesto-search"))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("the executable was rewritten in place; it must be a new file")
	}
	if after.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 755", after.Mode().Perm())
	}
	old := make([]byte, 64)
	n, _ := running.ReadAt(old, 0)
	if string(old[:n]) != "#!/bin/sh\necho old\n" {
		t.Fatalf("the open old file changed under its reader: %q", old[:n])
	}
}

// Data files keep the in-place copy whatever their mode bits say. SMB mounts
// report files as 0700 or 0755 and ignore chmod, so a mode-based guess would
// send every fact edit down the unlink-and-recreate path, where the fact is
// briefly absent from its own name.
func TestReplaceByCopyRewritesInPlaceEvenWithExecuteBits(t *testing.T) {
	t.Parallel()
	for _, mode := range []os.FileMode{0o644, 0o700, 0o755} {
		dir := t.TempDir()
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(dir, "fact.md")
		if err := os.WriteFile(dest, []byte("old\n"), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".tmp"), []byte("new\n"), mode); err != nil {
			t.Fatal(err)
		}
		before, err := os.Stat(dest)
		if err != nil {
			t.Fatal(err)
		}
		if err := replaceByCopy(root, ".tmp", "fact.md"); err != nil {
			t.Fatalf("mode %o: replaceByCopy: %v", mode, err)
		}
		after, err := os.Stat(dest)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before, after) {
			t.Fatalf("mode %o: a data file must be rewritten in place, not recreated", mode)
		}
		if got, _ := os.ReadFile(dest); string(got) != "new\n" {
			t.Fatalf("mode %o: content = %q", mode, got)
		}
		root.Close()
	}
}

// An unreadable temp must fail before the destination is truncated.
func TestReplaceByCopyLeavesDestinationIntactWhenTempUnreadable(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of mode")
	}
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	dest := filepath.Join(dir, "fact.md")
	if err := os.WriteFile(dest, []byte("the fact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tmp"), []byte("new\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := replaceByCopy(root, ".tmp", "fact.md"); err == nil {
		t.Fatal("expected an unreadable temp to fail")
	}
	if got, _ := os.ReadFile(dest); string(got) != "the fact\n" {
		t.Fatalf("destination damaged by a failed replace: %q", got)
	}
}

// MovePath refuses an occupied destination on the rename path too, so the fast
// path and the fallback agree.
func TestMovePathRefusesOccupiedDestination(t *testing.T) {
	t.Parallel()
	from := t.TempDir()
	to := t.TempDir()
	src := filepath.Join(from, "dec-a.md")
	dest := filepath.Join(to, "dec-a.md")
	if err := os.WriteFile(src, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := MovePath(src, dest)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("err = %v, want os.ErrExist", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "theirs\n" {
		t.Fatalf("destination overwritten: %q", got)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should survive: %v", err)
	}
}
