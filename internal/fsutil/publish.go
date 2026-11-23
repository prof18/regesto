// Package fsutil holds small filesystem helpers shared across Regesto packages.
package fsutil

import (
	"errors"
	"os"
	"syscall"
)

// PublishExclusive makes a complete same-directory temporary file visible at
// destName without replacing an existing target. Prefer a hard link so a race
// that creates destName after verification fails with EEXIST instead of
// overwriting. On filesystems that reject hard links (SMB/CIFS, some NFS, FAT),
// fall back to reserving destName with O_EXCL and renaming the temp over it,
// which keeps exclusive create semantics and publishes complete content in one
// rename (carrying the temp's mode across).
func PublishExclusive(root *os.Root, tmpName, destName string) error {
	err := root.Link(tmpName, destName)
	if err == nil {
		return nil
	}
	if !IsLinkUnsupported(err) {
		return err
	}
	if fallbackErr := exclusiveRenameFromTemp(root, tmpName, destName); fallbackErr != nil {
		return errors.Join(err, fallbackErr)
	}
	return nil
}

// IsLinkUnsupported reports whether err means this filesystem cannot create
// hard links. EEXIST and other real failures must stay false so callers keep
// refusing to overwrite.
//
// ENOTSUP and EOPNOTSUPP are identical on many Unixes but not all Darwin
// builds, so both are listed. EINVAL and EPERM cover CIFS/FAT refusals; the
// fallback still uses O_EXCL, so nothing can be overwritten.
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

// exclusiveRenameFromTemp reserves destName with O_EXCL, then renames the
// already-complete temp over that reservation. The empty reservation is never
// left readable as the published fact: either rename replaces it with the full
// temp inode, or the reservation is removed on failure.
func exclusiveRenameFromTemp(root *os.Root, tmpName, destName string) error {
	reserve, err := root.OpenFile(destName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := reserve.Close(); err != nil {
		_ = root.Remove(destName)
		return err
	}
	if err := root.Rename(tmpName, destName); err != nil {
		_ = root.Remove(destName)
		return err
	}
	return nil
}
