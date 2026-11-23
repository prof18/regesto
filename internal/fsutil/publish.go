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
// name (rename when the filesystem allows it; otherwise copy+sync into the
// reservation fd). Exclusive create semantics are preserved either way; the
// copy tier is not atomic — readers may observe an empty or partial file, and
// an interrupt mid-copy can leave a stub that later creates refuse with
// "already exists" until removed by hand.
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

// PublishReplace publishes a complete same-directory temporary file over an
// existing destName. Prefer rename. When rename fails (some SMB clients reject
// renameat and may delete the destination), write the temp into the destination
// by fd — recreating with O_EXCL if the failed rename removed the name — so the
// update does not depend on a working rename.
func PublishReplace(root *os.Root, tmpName, destName string) error {
	if err := root.Rename(tmpName, destName); err == nil {
		return nil
	}
	return replaceByCopy(root, tmpName, destName)
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
// already-complete temp into that name. Prefer rename-over-reservation. On
// rename failure, reopen only the reservation we created (SameFile) or, if the
// name was deleted by a broken renameat, reserve again with O_EXCL — never
// Remove a name we cannot prove we still own, and never reopen+O_TRUNC by name
// alone.
func exclusivePublishFromTemp(root *os.Root, tmpName, destName string) error {
	reserve, err := root.OpenFile(destName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	reserveInfo, err := reserve.Stat()
	if err != nil {
		_ = reserve.Close()
		_ = root.Remove(destName)
		return err
	}
	if err := reserve.Close(); err != nil {
		_ = root.Remove(destName)
		return err
	}
	if err := root.Rename(tmpName, destName); err == nil {
		return nil
	}

	dst, err := reopenOwnedReservation(root, destName, reserveInfo)
	if err != nil {
		return err
	}
	ownedInfo := reserveInfo
	if info, statErr := dst.Stat(); statErr == nil {
		ownedInfo = info
	}
	if err := copyTempIntoFD(root, tmpName, dst); err != nil {
		if stillOwned(root, destName, ownedInfo) {
			_ = root.Remove(destName)
		}
		return err
	}
	return nil
}

// reopenOwnedReservation returns a writable fd for destName only when it is
// still the inode we reserved, or a fresh O_EXCL fd when a failed rename
// deleted the name. If another writer holds the name, returns an exist error
// without removing anything.
func reopenOwnedReservation(root *os.Root, destName string, reserveInfo os.FileInfo) (*os.File, error) {
	f, err := root.OpenFile(destName, os.O_WRONLY, 0)
	if err == nil {
		info, statErr := f.Stat()
		if statErr != nil {
			_ = f.Close()
			return nil, statErr
		}
		if os.SameFile(info, reserveInfo) {
			if err := f.Truncate(0); err != nil {
				_ = f.Close()
				return nil, err
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				_ = f.Close()
				return nil, err
			}
			return f, nil
		}
		_ = f.Close()
		return nil, os.ErrExist
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return root.OpenFile(destName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

func stillOwned(root *os.Root, destName string, reserveInfo os.FileInfo) bool {
	info, err := root.Stat(destName)
	if err != nil {
		return false
	}
	return os.SameFile(info, reserveInfo)
}

func replaceByCopy(root *os.Root, tmpName, destName string) error {
	dst, err := root.OpenFile(destName, os.O_WRONLY, 0)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// Failed rename may have removed dest; publish exclusively from tmp.
		return exclusivePublishFromTemp(root, tmpName, destName)
	}
	if err := dst.Truncate(0); err != nil {
		_ = dst.Close()
		return err
	}
	if _, err := dst.Seek(0, io.SeekStart); err != nil {
		_ = dst.Close()
		return err
	}
	return copyTempIntoFD(root, tmpName, dst)
}

func copyTempIntoFD(root *os.Root, tmpName string, dst *os.File) (err error) {
	defer func() {
		if closeErr := dst.Close(); err == nil {
			err = closeErr
		}
	}()

	src, err := root.Open(tmpName)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	if err := dst.Sync(); err != nil {
		return err
	}
	if err := dst.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	return root.Remove(tmpName)
}
