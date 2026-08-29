package harvest

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func openHarvestRoot(path string) (*os.Root, string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, "", err
	}
	volume := filepath.VolumeName(resolved)
	anchorPath := volume + string(filepath.Separator)
	rel, err := filepath.Rel(anchorPath, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, "", fmt.Errorf("knowledge-base root %s is outside filesystem anchor %s", resolved, anchorPath)
	}
	root, err := os.OpenRoot(anchorPath)
	if err != nil {
		return nil, "", err
	}
	if rel == "." {
		return root, resolved, nil
	}
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		info, err := root.Lstat(component)
		if err != nil {
			root.Close()
			return nil, "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			root.Close()
			return nil, "", fmt.Errorf("knowledge-base root component %q is not a directory", component)
		}
		child, err := root.OpenRoot(component)
		if err != nil {
			root.Close()
			return nil, "", err
		}
		opened, err := child.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			child.Close()
			root.Close()
			return nil, "", fmt.Errorf("knowledge-base root component %q changed while opening", component)
		}
		root.Close()
		root = child
	}
	return root, resolved, nil
}

func validateHarvestOutputRoots(root *os.Root) error {
	for _, name := range []string{".state", "inbox"} {
		info, err := root.Lstat(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse symlink or non-directory knowledge-base output root %s", name)
		}
	}
	return nil
}

func openSafeDir(anchor *os.Root, path string, create bool) (*os.Root, error) {
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("refuse path outside knowledge-base root: %s", path)
	}
	root, err := anchor.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	if clean == "." {
		return root, nil
	}
	for _, component := range strings.Split(clean, string(filepath.Separator)) {
		info, statErr := root.Lstat(component)
		if os.IsNotExist(statErr) && create {
			if err := root.Mkdir(component, 0o755); err != nil && !os.IsExist(err) {
				root.Close()
				return nil, err
			}
			info, statErr = root.Lstat(component)
		}
		if statErr != nil {
			root.Close()
			return nil, statErr
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			root.Close()
			return nil, fmt.Errorf("refuse symlink or non-directory %q in knowledge-base output path", component)
		}
		child, err := root.OpenRoot(component)
		if err != nil {
			root.Close()
			return nil, err
		}
		opened, err := child.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			child.Close()
			root.Close()
			return nil, fmt.Errorf("knowledge-base output component %q changed while opening", component)
		}
		root.Close()
		root = child
	}
	return root, nil
}

func readRootFile(anchor *os.Root, path string) ([]byte, error) {
	root, err := openSafeDir(anchor, filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := filepath.Base(path)
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refuse non-regular knowledge-base state file %s", path)
	}
	return root.ReadFile(name)
}

func writeRootAtomic(anchor *os.Root, path string, data []byte, mode os.FileMode) error {
	root, err := openSafeDir(anchor, filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.Base(path)
	tmpName, err := rootTempName(".regesto-harvest-tmp-")
	if err != nil {
		return err
	}
	defer root.Remove(tmpName)
	file, err := root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return root.Rename(tmpName, name)
}

func writeRootExclusive(anchor *os.Root, path string, data []byte, mode os.FileMode) error {
	root, err := openSafeDir(anchor, filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.OpenFile(filepath.Base(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	name := filepath.Base(path)
	keep := false
	defer func() {
		if !keep {
			root.Remove(name)
		}
	}()
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	keep = true
	return nil
}

func rootTempName(prefix string) (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(random[:]), nil
}
