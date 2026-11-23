package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestIsLinkUnsupported(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"exist", os.ErrExist, false},
		{"enotsup", syscall.ENOTSUP, true},
		{"eopnotsupp distinct on darwin", syscall.EOPNOTSUPP, true},
		{"einval", syscall.EINVAL, true},
		{"eperm", syscall.EPERM, true},
		{"wrapped enotsup", &os.PathError{Op: "link", Path: "x", Err: syscall.ENOTSUP}, true},
		{"wrapped exist", &os.PathError{Op: "link", Path: "x", Err: syscall.EEXIST}, false},
		{"other", errors.New("nope"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsLinkUnsupported(tc.err); got != tc.want {
				t.Fatalf("IsLinkUnsupported(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestPublishExclusiveHardlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := os.WriteFile(filepath.Join(dir, "tmp"), []byte("fact-body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PublishExclusive(root, "tmp", "dest.md"); err != nil {
		t.Fatalf("PublishExclusive: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "dest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fact-body\n" {
		t.Fatalf("dest content = %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "tmp2"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = PublishExclusive(root, "tmp2", "dest.md")
	if err == nil {
		t.Fatal("expected exclusive publish to fail when dest exists")
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "dest.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "fact-body\n" {
		t.Fatalf("dest overwritten on conflict: %q (err=%v)", got, err)
	}
}

func TestExclusiveRenameFromTemp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	tmpPath := filepath.Join(dir, "tmp")
	if err := os.WriteFile(tmpPath, []byte("renamed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exclusiveRenameFromTemp(root, "tmp", "dest.md"); err != nil {
		t.Fatalf("exclusiveRenameFromTemp: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "dest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "renamed\n" {
		t.Fatalf("dest content = %q", got)
	}
	info, err := os.Stat(filepath.Join(dir, "dest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("dest mode = %o, want 0644", info.Mode().Perm())
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("temp should be gone after rename, stat err=%v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "tmp2"), []byte("other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = exclusiveRenameFromTemp(root, "tmp2", "dest.md")
	if err == nil {
		t.Fatal("expected O_EXCL failure when dest exists")
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "dest.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "renamed\n" {
		t.Fatalf("dest overwritten: %q (err=%v)", got, err)
	}
}

func TestPublishExclusiveOnSMB(t *testing.T) {
	base := os.Getenv("REGESTO_SMB_DIR")
	if base == "" {
		t.Skip("set REGESTO_SMB_DIR to a directory on an SMB mount to run this test")
	}
	info, err := os.Stat(base)
	if err != nil {
		t.Fatalf("REGESTO_SMB_DIR %q: %v", base, err)
	}
	if !info.IsDir() {
		t.Fatalf("REGESTO_SMB_DIR %q is not a directory", base)
	}

	dir, err := os.MkdirTemp(base, "regesto-smb-publish-*")
	if err != nil {
		t.Fatalf("MkdirTemp on SMB: %v", err)
	}
	defer os.RemoveAll(dir)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	tmpPath := filepath.Join(dir, "tmp")
	if err := os.WriteFile(tmpPath, []byte("smb-body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		t.Fatal(err)
	}

	linkErr := root.Link("tmp", "link-probe")
	if linkErr == nil {
		t.Fatal("expected hard link to fail on SMB mount; Link succeeded")
	}
	if !IsLinkUnsupported(linkErr) {
		t.Fatalf("expected IsLinkUnsupported Link error on SMB, got %v", linkErr)
	}
	t.Logf("Link unsupported as expected: %v", linkErr)

	if err := PublishExclusive(root, "tmp", "dest.md"); err != nil {
		t.Fatalf("PublishExclusive on SMB: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "dest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "smb-body\n" {
		t.Fatalf("dest content = %q", got)
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("temp should be gone after publish, stat err=%v", err)
	}
	t.Log("exclusive publish (rename or copy into O_EXCL reservation) succeeded on SMB")

	if err := os.WriteFile(filepath.Join(dir, "tmp2"), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = PublishExclusive(root, "tmp2", "dest.md")
	if err == nil {
		t.Fatal("expected exclusive publish to fail when dest exists on SMB")
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "dest.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "smb-body\n" {
		t.Fatalf("dest overwritten on SMB: %q (err=%v)", got, err)
	}
}

func TestExclusiveRenameFromTempMode600(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	tmpPath := filepath.Join(dir, "tmp")
	if err := os.WriteFile(tmpPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exclusiveRenameFromTemp(root, "tmp", "dest.md"); err != nil {
		t.Fatalf("exclusiveRenameFromTemp: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "dest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("dest mode = %o, want 0600", info.Mode().Perm())
	}
}
