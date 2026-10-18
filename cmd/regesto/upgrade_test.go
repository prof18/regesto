package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestUpgradeInstanceMutationsDoNotFollowEscapingAncestor(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "adapters")); err != nil {
		t.Fatal(err)
	}
	rel := "adapters/skills/foreign/SKILL.md"
	outsideFile := filepath.Join(outside, "skills", "foreign", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(outsideFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideFile, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backupFile(root, rel); err == nil {
		t.Fatal("backup followed an escaping ancestor")
	}
	if err := writeInstanceFile(root, rel, []byte("engine\n")); err == nil {
		t.Fatal("write followed an escaping ancestor")
	}
	if err := removeInstanceFile(root, rel); err == nil {
		t.Fatal("remove followed an escaping ancestor")
	}
	body, err := os.ReadFile(outsideFile)
	if err != nil || string(body) != "outside\n" {
		t.Fatalf("outside file changed: body=%q err=%v", body, err)
	}
}

func TestUpgradeBackupsAreExclusiveAndPreserveMode(t *testing.T) {
	root := t.TempDir()
	rel := "SCHEMA.md"
	path := filepath.Join(root, rel)
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := backupFile(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	second, err := backupFile(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("backup collision at %s", first)
	}
	for _, backup := range []string{first, second} {
		body, err := os.ReadFile(backup)
		if err != nil || string(body) != "original\n" {
			t.Fatalf("backup %s: body=%q err=%v", backup, body, err)
		}
		info, err := os.Stat(backup)
		if err != nil {
			t.Fatalf("stat backup %s: %v", backup, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("backup %s: mode=%v", backup, info.Mode().Perm())
		}
	}
}

func TestUpgradeWritesAndBackupsRestoreModeAfterUmask(t *testing.T) {
	root := t.TempDir()
	oldUmask := syscall.Umask(0o077)
	defer syscall.Umask(oldUmask)

	if err := writeInstanceFile(root, "SCHEMA.md", []byte("schema\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeInstanceFile(root, "bin/regesto-search", []byte("#!/bin/sh\n")); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(root, "SCHEMA.md"):          0o644,
		filepath.Join(root, "bin/regesto-search"): 0o755,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %o, want %o", path, got, want)
		}
	}
	backup, err := backupFile(root, "SCHEMA.md")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("backup mode = %o, want 644", got)
	}
}

func TestRefreshInstanceEngineReplacesStaleCopy(t *testing.T) {
	root, tools := t.TempDir(), t.TempDir()
	exe := filepath.Join(tools, "regesto")
	if err := os.WriteFile(exe, []byte("new engine\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, "bin", "regesto")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old engine\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A dry run reports the replacement without making it.
	refreshed, err := refreshInstanceEngine(root, exe, true)
	if err != nil || !refreshed {
		t.Fatalf("dry run: refreshed=%v err=%v", refreshed, err)
	}
	if body, _ := os.ReadFile(stale); string(body) != "old engine\n" {
		t.Fatalf("dry run wrote the engine: %q", body)
	}

	refreshed, err = refreshInstanceEngine(root, exe, false)
	if err != nil || !refreshed {
		t.Fatalf("refresh: refreshed=%v err=%v", refreshed, err)
	}
	body, err := os.ReadFile(stale)
	if err != nil || string(body) != "new engine\n" {
		t.Fatalf("instance engine: body=%q err=%v", body, err)
	}
	if info, err := os.Stat(stale); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("instance engine is not executable: %v %v", info, err)
	}

	// Once current, there is nothing to do.
	if refreshed, err := refreshInstanceEngine(root, exe, false); err != nil || refreshed {
		t.Fatalf("second refresh: refreshed=%v err=%v", refreshed, err)
	}
}

func TestRefreshInstanceEngineLeavesOtherCasesAlone(t *testing.T) {
	tools := t.TempDir()
	exe := filepath.Join(tools, "regesto")
	if err := os.WriteFile(exe, []byte("new engine\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// No binary in the instance: none is created.
	empty := t.TempDir()
	if refreshed, err := refreshInstanceEngine(empty, exe, false); err != nil || refreshed {
		t.Fatalf("empty instance: refreshed=%v err=%v", refreshed, err)
	}
	if _, err := os.Stat(filepath.Join(empty, "bin", "regesto")); !os.IsNotExist(err) {
		t.Fatalf("an engine was created in an instance that had none: %v", err)
	}

	// A source checkout rebuilds its own engine; upgrade must not overwrite it.
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, "go.mod"), []byte("module fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	built := filepath.Join(checkout, "bin", "regesto")
	if err := os.MkdirAll(filepath.Dir(built), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(built, []byte("checkout build\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if refreshed, err := refreshInstanceEngine(checkout, exe, false); err != nil || refreshed {
		t.Fatalf("checkout: refreshed=%v err=%v", refreshed, err)
	}
	if body, _ := os.ReadFile(built); string(body) != "checkout build\n" {
		t.Fatalf("checkout engine overwritten: %q", body)
	}

	// A symlink is the user's choice of engine; it is not followed or replaced.
	linked, target := t.TempDir(), filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte("linked engine\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(linked, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(linked, "bin", "regesto")); err != nil {
		t.Fatal(err)
	}
	if refreshed, err := refreshInstanceEngine(linked, exe, false); err != nil || refreshed {
		t.Fatalf("symlink: refreshed=%v err=%v", refreshed, err)
	}
	if body, _ := os.ReadFile(target); string(body) != "linked engine\n" {
		t.Fatalf("symlink target overwritten: %q", body)
	}
}
