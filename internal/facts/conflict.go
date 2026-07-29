package facts

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Conflict is one sync-conflict copy and the file it conflicts with.
type Conflict struct {
	// ConflictPath is the copy the sync client wrote, relative to the KB root.
	ConflictPath string
	// BasePath is the file it conflicts with, relative to the KB root. Empty
	// when the original is gone — a delete-versus-edit conflict.
	BasePath string
}

// BaseName strips the conflict marker, turning
// dec-a.sync-conflict-20260729-101500-ABCDEF.md back into dec-a.md.
func BaseName(name string) string {
	i := strings.Index(name, ConflictMarker)
	if i < 0 {
		return name
	}
	return name[:i] + filepath.Ext(name)
}

// FindConflicts lists sync-conflict copies under knowledge/.
//
// It covers all of knowledge/, not just facts/: a conflict on a generated topic
// page matters less, since those are rebuilt, but a stray copy would still sit
// in the vault confusing a human reader.
func FindConflicts(kbRoot string) ([]Conflict, error) {
	root := filepath.Join(kbRoot, "knowledge")
	var out []Conflict
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !IsConflict(d.Name()) {
			return nil
		}
		rel, relErr := filepath.Rel(kbRoot, path)
		if relErr != nil {
			return nil
		}
		c := Conflict{ConflictPath: filepath.ToSlash(rel)}

		base := filepath.Join(filepath.Dir(path), BaseName(d.Name()))
		if _, statErr := os.Stat(base); statErr == nil {
			if r, err := filepath.Rel(kbRoot, base); err == nil {
				c.BasePath = filepath.ToSlash(r)
			}
		}
		out = append(out, c)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
