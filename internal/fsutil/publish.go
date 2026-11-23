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
// fall back to reserving destName with O_EXCL and publishing the temp into that
// name (rename when the filesystem allows it; otherwise copy+sync into a fresh
// reservation). Exclusive create semantics are preserved either way.
func PublishExclusive(root *os.Root, tmpName, destName string) error {
	err := root.Link(tmpName, destName)
	if err == nil {
		return nil
	}
	if !IsLinkUnsupported(err) {
		return err
	}
	if fallbackErr := exclusivePublishFromTemp(root, tmpName, destName); fallbackErr != nil {
		return errors.Join(err, fallbackErr)
	}
	return nil
}

// IsLinkUnsupported reports whether err means this filesystem cannot create
// hard links. EEXIST and other real failures must stay false so callers keep
// refusing to overwrite.
//
// ENOTSUP and EOPNOTSUPP are identical on Linux, distinct on Darwin, so both
// are listed. EINVAL and EPERM cover CIFS/FAT refusals; the fallback still
// uses O_EXCL, so nothing can be overwritten.
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

// exclusivePublishFromTemp reserves destName with O_EXCL, then publishes the
// already-complete temp into that name. Prefer rename-over-reservation so the
// empty file is never left as the published fact. Some SMB clients reject
// renameat and, on failure, delete the destination while leaving the source;
// recover by reserving again and copy+sync into it. On any failure the
// reservation is removed so nothing corrupt remains.
func exclusivePublishFromTemp(root *os.Root, tmpName, destName string) error {
	if err := reserveExclusive(root, destName); err != nil {
		return err
	}
	if err := root.Rename(tmpName, destName); err == nil {
		return nil
	}
	// Failed rename may have removed destName (observed on macOS SMB).
	_ = root.Remove(destName)
	if err := reserveExclusive(root, destName); err != nil {
		return err
	}
	if err := copyTempIntoReserved(root, tmpName, destName); err != nil {
		_ = root.Remove(destName)
		return err
	}
	return nil
}

// exclusiveRenameFromTemp is the exclusive publish used by tests that exercise
// the hard-link fallback path.
func exclusiveRenameFromTemp(root *os.Root, tmpName, destName string) error {
	return exclusivePublishFromTemp(root, tmpName, destName)
}

func reserveExclusive(root *os.Root, destName string) error {
	reserve, err := root.OpenFile(destName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := reserve.Close(); err != nil {
		_ = root.Remove(destName)
		return err
	}
	return nil
}

func copyTempIntoReserved(root *os.Root, tmpName, destName string) error {
	src, err := root.Open(tmpName)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}

	dst, err := root.OpenFile(destName, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	if err := root.Chmod(destName, info.Mode().Perm()); err != nil {
		return err
	}
	return root.Remove(tmpName)
}
