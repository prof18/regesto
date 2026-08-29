package install

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Result struct {
	Applied int      `json:"applied"`
	Backups []string `json:"backups"`
}

// Apply executes a previously reviewed plan. Each target is resolved again
// immediately before its mutation, and changed input files are refused.
func Apply(plan *Plan) (Result, error) {
	result := Result{Backups: make([]string, 0)}
	for i := range plan.Items {
		item := &plan.Items[i]
		if !mutates(item.Action) {
			continue
		}
		for _, declared := range item.DeclaredTargets {
			var got string
			var err error
			if item.Kind == "skill-link" || item.Kind == "engine-link" {
				got, err = canonicalLinkPath(declared)
			} else {
				got, err = CanonicalTarget(declared)
			}
			if err != nil {
				return result, err
			}
			if got != item.CanonicalTarget {
				return result, fmt.Errorf("refuse %s: target changed after planning: %s resolved to %s, planned %s", item.ID, declared, got, item.CanonicalTarget)
			}
		}
		switch item.Kind {
		case "skills-directory":
			if err := secureMkdirAll(item.CanonicalTarget); err != nil {
				return result, err
			}
		case "skill-link", "engine-link":
			if err := applyLink(*item); err != nil {
				return result, err
			}
		case "skill-render-prune":
			{
				stage, err := CanonicalTarget(filepath.Join(plan.KBRoot, ".state", "skills"))
				if err != nil {
					return result, err
				}
				if !within(stage, item.CanonicalTarget) {
					return result, fmt.Errorf("refuse generated-stage removal outside %s: %s", stage, item.CanonicalTarget)
				}
				root, err := os.OpenRoot(stage)
				if err != nil {
					return result, err
				}
				rel, err := filepath.Rel(stage, item.CanonicalTarget)
				if err != nil {
					root.Close()
					return result, err
				}
				marker, err := root.ReadFile(filepath.Join(rel, ".regesto-owned"))
				if err != nil || !bytes.Equal(marker, item.ownership) {
					root.Close()
					return result, fmt.Errorf("refuse %s: generated-stage ownership changed after planning", item.ID)
				}
				digest, err := treeDigest(item.CanonicalTarget)
				if err != nil || !bytes.Equal(digest, item.before) {
					root.Close()
					return result, fmt.Errorf("refuse %s: generated-stage contents changed after planning", item.ID)
				}
				if err := root.RemoveAll(rel); err != nil {
					root.Close()
					return result, err
				}
				if err := root.Close(); err != nil {
					return result, err
				}
				break
			}
		case "skill-render-file":
			stage, err := CanonicalTarget(filepath.Join(plan.KBRoot, ".state", "skills"))
			if err != nil {
				return result, err
			}
			if !within(stage, item.CanonicalTarget) {
				return result, fmt.Errorf("refuse generated-file removal outside %s: %s", stage, item.CanonicalTarget)
			}
			root, err := os.OpenRoot(stage)
			if err != nil {
				return result, err
			}
			rel, err := filepath.Rel(stage, item.CanonicalTarget)
			if err != nil {
				root.Close()
				return result, err
			}
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) < 2 {
				root.Close()
				return result, fmt.Errorf("refuse generated-file removal without skill directory: %s", rel)
			}
			marker, err := root.ReadFile(filepath.Join(parts[0], ".regesto-owned"))
			if err != nil || !bytes.Equal(marker, item.ownership) {
				root.Close()
				return result, fmt.Errorf("refuse %s: generated-stage ownership changed after planning", item.ID)
			}
			body, err := root.ReadFile(rel)
			if err != nil || !bytes.Equal(body, item.before) {
				root.Close()
				return result, fmt.Errorf("refuse %s: generated file changed after planning", item.ID)
			}
			if err := root.Remove(rel); err != nil {
				root.Close()
				return result, err
			}
			if err := root.Close(); err != nil {
				return result, err
			}
		case "skill-render":
			if err := applyFile(*item, false, &result); err != nil {
				return result, err
			}
		case "instructions", "hook":
			if err := applyFile(*item, item.Action == "update", &result); err != nil {
				return result, err
			}
		default:
			return result, fmt.Errorf("unknown install item kind %q", item.Kind)
		}
		result.Applied++
	}
	return result, nil
}

func verifyRootBefore(root *os.Root, name string, item Item) error {
	got, err := root.ReadFile(name)
	if item.before == nil {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("refuse %s: target appeared after planning", item.ID)
	}
	if err != nil {
		return fmt.Errorf("refuse %s: target changed after planning: %w", item.ID, err)
	}
	if !bytes.Equal(got, item.before) {
		return fmt.Errorf("refuse %s: target contents changed after planning", item.ID)
	}
	return nil
}

func applyLink(item Item) error {
	root, name, err := openTargetRoot(item.CanonicalTarget)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if item.Action == "create" {
		if err == nil {
			return fmt.Errorf("refuse %s: target appeared after planning", item.ID)
		}
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		if err != nil {
			return fmt.Errorf("refuse %s: owned link changed after planning: %w", item.ID, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refuse %s: owned link is no longer a symlink", item.ID)
		}
		raw, err := root.Readlink(name)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(raw) {
			raw = filepath.Join(filepath.Dir(item.CanonicalTarget), raw)
		}
		if filepath.Clean(raw) != string(item.before) {
			return fmt.Errorf("refuse %s: owned link target changed after planning", item.ID)
		}
		if item.Action == "remove" {
			return root.Remove(name)
		}
	}
	if item.Action == "create" {
		return root.Symlink(item.linkTarget, name)
	}
	tmpName, err := randomTempName(".regesto-link-")
	if err != nil {
		return err
	}
	defer root.Remove(tmpName)
	if err := root.Symlink(item.linkTarget, tmpName); err != nil {
		return err
	}
	// Recheck through the already-open parent immediately before publication.
	raw, err := root.Readlink(name)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(filepath.Dir(item.CanonicalTarget), raw)
	}
	if filepath.Clean(raw) != string(item.before) {
		return fmt.Errorf("refuse %s: owned link target changed before publication", item.ID)
	}
	return root.Rename(tmpName, name)
}

func applyFile(item Item, withBackup bool, result *Result) error {
	if err := verifyItemOwnership(item); err != nil {
		return err
	}
	root, name, err := openTargetRoot(item.CanonicalTarget)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := verifyRootBefore(root, name, item); err != nil {
		return err
	}
	if withBackup {
		backupName, err := backupRoot(root, name, item.before, item.mode)
		if err != nil {
			return err
		}
		result.Backups = append(result.Backups, filepath.Join(filepath.Dir(item.CanonicalTarget), backupName))
	}
	tmp, tmpName, err := createRootTemp(root, ".regesto-install-")
	if err != nil {
		return err
	}
	defer root.Remove(tmpName)
	if _, err := tmp.Write(item.desired); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(item.mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := verifyRootBefore(root, name, item); err != nil {
		return err
	}
	if err := verifyItemOwnership(item); err != nil {
		return err
	}
	if item.before == nil {
		// A hard link publishes without replacing a target that appeared after
		// verification. Removing the temporary name leaves the linked inode live.
		return root.Link(tmpName, name)
	}
	return root.Rename(tmpName, name)
}

func verifyItemOwnership(item Item) error {
	if len(item.ownership) == 0 {
		return nil
	}
	root, err := os.OpenRoot(filepath.Dir(item.ownershipTarget))
	if err != nil {
		return fmt.Errorf("refuse %s: ownership marker changed after planning: %w", item.ID, err)
	}
	defer root.Close()
	marker, err := root.ReadFile(filepath.Base(item.ownershipTarget))
	if err != nil || !bytes.Equal(marker, item.ownership) {
		return fmt.Errorf("refuse %s: ownership marker changed after planning", item.ID)
	}
	return nil
}

func backupRoot(root *os.Root, name string, body []byte, mode os.FileMode) (string, error) {
	base := filepath.Base(name) + ".regesto-backup." + time.Now().UTC().Format("20060102T150405.000000000Z")
	for attempt := 0; attempt < 100; attempt++ {
		candidate := base
		if attempt > 0 {
			candidate = fmt.Sprintf("%s.%d", base, attempt)
		}
		f, err := root.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(body); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		return candidate, nil
	}
	return "", fmt.Errorf("could not create a unique backup beside %s", name)
}

func openTargetRoot(target string) (*os.Root, string, error) {
	parent := filepath.Dir(target)
	if err := secureMkdirAll(parent); err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, "", err
	}
	return root, filepath.Base(target), nil
}

func secureMkdirAll(path string) error {
	cur := filepath.Clean(path)
	var suffix []string
	for {
		info, err := os.Stat(cur)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("install parent %s is not a directory", cur)
			}
			root, err := os.OpenRoot(cur)
			if err != nil {
				return err
			}
			defer root.Close()
			if len(suffix) > 0 {
				rel := strings.Join(reverseCopy(suffix), string(filepath.Separator))
				if err := root.MkdirAll(rel, 0o755); err != nil {
					return err
				}
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return fmt.Errorf("install parent %s has no existing ancestor", path)
		}
		suffix = append(suffix, filepath.Base(cur))
		cur = parent
	}
}

func reverseCopy(values []string) []string {
	out := make([]string, len(values))
	for i := range values {
		out[len(values)-1-i] = values[i]
	}
	return out
}

func createRootTemp(root *os.Root, prefix string) (*os.File, string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		name, err := randomTempName(prefix)
		if err != nil {
			return nil, "", err
		}
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, name, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("could not allocate a unique install temporary file")
}

func randomTempName(prefix string) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(random[:]), nil
}
