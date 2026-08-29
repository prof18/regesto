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
