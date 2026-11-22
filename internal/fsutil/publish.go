// Package fsutil holds small filesystem helpers shared across Regesto packages.
package fsutil

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// PublishExclusive makes a complete same-directory temporary file visible at
// destName without replacing an existing target. Prefer a hard link so a race
// that creates destName after verification fails with EEXIST instead of
// overwriting. On filesystems that reject hard links (SMB/CIFS, some NFS, FAT),
// fall back to O_CREAT|O_EXCL create and copy, which preserves the same
// exclusive semantics.
func PublishExclusive(root *os.Root, tmpName, destName string) error {
	err := root.Link(tmpName, destName)
	if err == nil {
		return nil
	}
	if !IsLinkUnsupported(err) {
		return err
	}
	return exclusiveCopyFromTemp(root, tmpName, destName)
}

// IsLinkUnsupported reports whether err means this filesystem cannot create
// hard links. EEXIST and other real failures must stay false so callers keep
// refusing to overwrite.
func IsLinkUnsupported(err error) bool {
	for _, code := range []syscall.Errno{
		syscall.ENOTSUP,
		syscall.EOPNOTSUPP,
		syscall.EINVAL,
		syscall.EPERM,
	} {
		if errors.Is(err, code) {
			return true
		}
	}
	return false
}

func exclusiveCopyFromTemp(root *os.Root, tmpName, destName string) error {
	info, err := root.Stat(tmpName)
	if err != nil {
		return err
	}
	src, err := root.Open(tmpName)
	if err != nil {
		return err
	}
	defer src.Close()

	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	dst, err := root.OpenFile(destName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = dst.Close()
			_ = root.Remove(destName)
		}
	}()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	if err := dst.Sync(); err != nil {
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	committed = true
	return nil
}
