// Package fsutil holds small filesystem helpers shared across Regesto packages.
package fsutil

import (
	"errors"
	"fmt"
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

// exclusivePublishFromTemp reserves destName with O_EXCL and keeps that fd.
// Prefer rename-over-reservation while the fd is held. On rename failure,
// decide from the held fd's link count (not from SameFile through the
// directory — Linux recycles inode numbers promptly): Nlink == 0 means the
// name was deleted, so close and O_EXCL again; Nlink > 0 means write into
// the held fd. Never Remove a name without the held inode still being linked.
func exclusivePublishFromTemp(root *os.Root, tmpName, destName string) error {
	reserve, err := root.OpenFile(destName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := root.Rename(tmpName, destName); err == nil {
		_ = reserve.Close()
		return nil
	}

	nlink, err := fileNlink(reserve)
	if err != nil {
		_ = reserve.Close()
		return err
	}
	if nlink == 0 {
		// Failed rename deleted the name (observed on macOS SMB). The held
		// inode is orphaned — do not write into it. Fresh O_EXCL: EEXIST if
		// another writer already claimed the name.
		_ = reserve.Close()
		reserve, err = root.OpenFile(destName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
	} else if err := prepareHeldReservation(reserve); err != nil {
		_ = reserve.Close()
		_ = root.Remove(destName)
		return err
	}

	if err := copyTempIntoFD(root, tmpName, reserve); err != nil {
		// We held an O_EXCL claim on destName; remove our failed stub. If a
		// racer replaced the name after Nlink dropped to 0 we never reached
		// copy (fresh O_EXCL would have failed with ErrExist).
		_ = root.Remove(destName)
		return err
	}
	return nil
}

func prepareHeldReservation(reserve *os.File) error {
	if err := reserve.Truncate(0); err != nil {
		return err
	}
	_, err := reserve.Seek(0, io.SeekStart)
	return err
}

func fileNlink(f *os.File) (uint64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("stat for %s: no syscall.Stat_t", f.Name())
	}
	return uint64(st.Nlink), nil
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
	if err := prepareHeldReservation(dst); err != nil {
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
	info, err := src.Stat()
	if err != nil {
		_ = src.Close()
		return err
	}
	_, copyErr := io.Copy(dst, src)
	// Close before Remove: some SMB clients return EBUSY while the temp is open.
	if closeErr := src.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}
	if err := dst.Sync(); err != nil {
		return err
	}
	if err := dst.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	return root.Remove(tmpName)
}
