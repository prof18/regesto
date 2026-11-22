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
		{"eopnotsupp", syscall.EOPNOTSUPP, true},
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

func TestExclusiveCopyFromTemp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if err := os.WriteFile(filepath.Join(dir, "tmp"), []byte("copied\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exclusiveCopyFromTemp(root, "tmp", "dest.md"); err != nil {
		t.Fatalf("exclusiveCopyFromTemp: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "dest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "copied\n" {
		t.Fatalf("dest content = %q", got)
	}
	info, err := os.Stat(filepath.Join(dir, "dest.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("dest mode = %o, want 0600", info.Mode().Perm())
	}

	err = exclusiveCopyFromTemp(root, "tmp", "dest.md")
	if err == nil {
		t.Fatal("expected O_EXCL failure when dest exists")
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "dest.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "copied\n" {
		t.Fatalf("dest overwritten: %q (err=%v)", got, err)
	}
}
