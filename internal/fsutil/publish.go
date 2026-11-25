// Package fsutil holds small filesystem helpers shared across Regesto packages.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
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
// update does not depend on a working rename. That copy is not atomic: a reader
// can catch it mid-write, and a copy that fails part-way leaves the
// destination partial while the caller's temp still holds the full content.
func PublishReplace(root *os.Root, tmpName, destName string) error {
	return publishReplace(root, tmpName, destName, false)
}

// PublishReplaceExecutable is PublishReplace for a binary or script. Its
// fallback never writes into the old file: a running binary is paged from its
// file (macOS kills it on an invalid code page; Linux refuses the open with
// ETXTBSY), and a shell reads its script incrementally while running it. The
// old name is unlinked instead, so anything still running keeps the old inode,
// and the content is published as a fresh file. The name is briefly absent, so
// this is only for files whose callers can fall back when it is missing.
//
// The caller has to say a file is executable. Mode bits cannot: SMB mounts
// commonly report every file as 0700 or 0755 and ignore chmod.
func PublishReplaceExecutable(root *os.Root, tmpName, destName string) error {
	return publishReplace(root, tmpName, destName, true)
}

func publishReplace(root *os.Root, tmpName, destName string, executable bool) error {
	renameErr := root.Rename(tmpName, destName)
	if renameErr == nil {
		return nil
	}
	fallback := replaceByCopy
	if executable {
		fallback = replaceByNewFile
	}
	if fallbackErr := fallback(root, tmpName, destName); fallbackErr != nil {
		// Keep the rename error: it names why the fast path was refused,
		// which the fallback's own error does not.
		return errors.Join(renameErr, fallbackErr)
	}
	return nil
}

// PublishReplacePath is PublishReplace for the callers that hold plain paths
// rather than an open root. Both paths must name files in one directory, which
// is what every temp-then-publish caller already does; rooting the operation
// there keeps the fallback's exclusive re-create in the same directory as the
// destination.
func PublishReplacePath(tmpPath, destPath string) error {
	dir := filepath.Dir(destPath)
	if filepath.Dir(tmpPath) != dir {
		return fmt.Errorf("publish %s over %s: both must be in one directory", tmpPath, destPath)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return PublishReplace(root, filepath.Base(tmpPath), filepath.Base(destPath))
}

// MovePath relocates an existing file into another directory. The publish
// helpers cannot: they take a temporary file beside its destination, and the
// fallback's exclusive re-create has to land in the destination's own
// directory. Lint's repairs genuinely move a fact — into the archive, or out
// of an alias project directory into the canonical one — so they need this.
//
// An occupied destPath is refused (os.ErrExist) rather than replaced. Prefer
// rename. Where rename is rejected, copy into the destination directory under
// an exclusive create, then remove the source; if the source cannot be removed
// the copy is deleted again, so a half-move does not strand one fact under two
// paths — for a scope move, one id in two project directories. If even that
// undo fails, the returned error says so.
func MovePath(srcPath, destPath string) error {
	// rename(2) would silently replace an occupied destination, which the
	// fallback refuses; check first so both paths agree. The check is not
	// atomic with the rename — there is no portable no-replace rename in the
	// standard library — but it closes everything short of a racing writer.
	if _, err := os.Lstat(destPath); err == nil {
		return fmt.Errorf("move %s to %s: %w", srcPath, destPath, os.ErrExist)
	} else if !os.IsNotExist(err) {
		return err
	}
	renameErr := os.Rename(srcPath, destPath)
	if renameErr == nil {
		return nil
	}
	if fallbackErr := moveByCopy(srcPath, destPath); fallbackErr != nil {
		return errors.Join(renameErr, fallbackErr)
	}
	return nil
}

func moveByCopy(srcPath, destPath string) error {
	if err := CopyPath(srcPath, destPath); err != nil {
		return err
	}
	if err := os.Remove(srcPath); err != nil {
		if rbErr := os.Remove(destPath); rbErr != nil {
			return errors.Join(err, fmt.Errorf("undo the copy at %s: %w", destPath, rbErr))
		}
		return err
	}
	return nil
}

// CopyPath copies an existing file into destPath, which may be in another
// directory and must not exist yet: the name is claimed exclusively, so a file
// another writer put there is refused rather than overwritten. The source is
// left in place. Conflict resolution archives the displaced fact this way, so
// the fact stays live until its replacement has landed.
func CopyPath(srcPath, destPath string) error {
	root, err := os.OpenRoot(filepath.Dir(destPath))
	if err != nil {
		return err
	}
	defer root.Close()

	tmpName, err := copyIntoRoot(root, srcPath)
	if err != nil {
		return err
	}
	// A successful hard link leaves the temporary name behind; the copy tier
	// consumes it, and removing a name that is already gone is harmless.
	defer func() { _ = root.Remove(tmpName) }()

	return PublishExclusive(root, tmpName, filepath.Base(destPath))
}

// copyIntoRoot writes srcPath's contents and permissions into a fresh
// temporary file inside root, and returns its name for publication.
func copyIntoRoot(root *os.Root, srcPath string) (string, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return "", err
	}

	name := fmt.Sprintf(".regesto-move-%d-%d", os.Getpid(), time.Now().UnixNano())
	tmp, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	finish := func(err error) (string, error) {
		_ = tmp.Close()
		_ = root.Remove(name)
		return "", err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		return finish(err)
	}
	if err := tmp.Sync(); err != nil {
		return finish(err)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return finish(err)
	}
	if err := tmp.Close(); err != nil {
		_ = root.Remove(name)
		return "", err
	}
	return name, nil
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

func replaceByCopy(root *os.Root, tmpName, destName string) error {
	// Open the temp before touching the destination: an unreadable temp must
	// fail while the destination still holds its old content, not after it
	// has been truncated.
	probe, err := root.Open(tmpName)
	if err != nil {
		return err
	}
	_ = probe.Close()

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

func replaceByNewFile(root *os.Root, tmpName, destName string) error {
	if err := root.Remove(destName); err != nil && !os.IsNotExist(err) {
		return err
	}
	return exclusivePublishFromTemp(root, tmpName, destName)
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
