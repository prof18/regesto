// Package manifest records which engine last wrote an instance's engine-owned
// files, and what they contained at that moment.
//
// Without it, upgrading is a guess: a file that differs from the engine's copy
// might be one the engine changed since, or one the user edited on purpose, and
// overwriting the second kind silently is the worst thing an upgrade can do. The
// hash of what was last written distinguishes them exactly.
//
// The file lives at the knowledge-base root and is replicated like the files it
// describes — it is a property of the instance, not of one machine.
package manifest

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileName is the manifest's name at the KB root. Dotted so markdown vaults and
// file browsers leave it alone; it is machine-written and never hand-edited.
const FileName = ".regesto-manifest"

type Manifest struct {
	Engine  string            // version string of the engine that last wrote these
	Written time.Time         // when
	Files   map[string]string // path relative to the KB root → sha256 hex
}

// Load reads the manifest. A missing one is not an error: instances created
// before manifests existed are the case upgrade has to handle most carefully,
// and an empty manifest is exactly the right description of them.
func Load(root string) (*Manifest, error) {
	m := &Manifest{Files: map[string]string{}}
	f, err := os.Open(filepath.Join(root, FileName))
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rest, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		switch key {
		case "engine":
			m.Engine = rest
		case "written":
			if t, err := time.Parse(time.RFC3339, rest); err == nil {
				m.Written = t
			}
		default:
			// "<sha256>  <path>", the shasum layout, so `shasum -c` can check it.
			m.Files[rest] = key
		}
	}
	return m, sc.Err()
}

// Save writes the manifest atomically. A half-written manifest would make every
// file look modified on the next upgrade, which is the failure that turns a
// routine upgrade into a wall of false warnings.
func Save(root string, m *Manifest) error {
	var b strings.Builder
	b.WriteString("# Written by `regesto init` and `regesto upgrade`. Machine-written — do not edit.\n")
	b.WriteString("# Records the engine that last wrote the files below and what they held then,\n")
	b.WriteString("# so an upgrade can tell an untouched file from one you changed on purpose.\n\n")
	fmt.Fprintf(&b, "engine %s\n", m.Engine)
	fmt.Fprintf(&b, "written %s\n\n", m.Written.UTC().Format(time.RFC3339))

	paths := make([]string, 0, len(m.Files))
	for p := range m.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&b, "%s  %s\n", m.Files[p], p)
	}

	final := filepath.Join(root, FileName)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// Sum is the hash recorded for a file's contents.
func Sum(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// Status is what an upgrade should do with one engine-owned file.
type Status int

const (
	Current  Status = iota // identical to the engine's copy; nothing to do
	Missing                // the engine ships it, the instance does not have it
	Stale                  // differs from the engine, matches what was last written — safe to replace
	Modified               // differs from both: edited locally since it was written
	Unknown                // differs from the engine, and nothing recorded what it should have been
	// The engine no longer ships it, and the copy here is byte for byte what
	// was last written — so removing it destroys nothing.
	Withdrawn
	// The engine no longer ships it, but this copy was changed since. The
	// change is someone's work and outlives the file's place in the engine.
	WithdrawnEdited
)

func (s Status) String() string {
	switch s {
	case Current:
		return "current"
	case Missing:
		return "missing"
	case Stale:
		return "stale"
	case Modified:
		return "modified"
	case Withdrawn:
		return "withdrawn"
	case WithdrawnEdited:
		return "withdrawn, edited"
	default:
		return "unknown"
	}
}

// Writable reports whether an upgrade may write this file without being forced.
func (s Status) Writable() bool { return s == Missing || s == Stale }

// Removable reports whether an upgrade may delete this file without being
// forced. Only the case where the contents are provably the engine's own.
func (s Status) Removable() bool { return s == Withdrawn }

type Change struct {
	Path   string
	Status Status
	Body   []byte // the engine's copy, for the statuses that get written
}

// PlanRemovals finds files this engine no longer ships but a previous one wrote
// here — a skill that was retired, a hook that moved. Left in place they keep
// working: the installer goes on rendering and linking a retired skill, so
// agents keep following instructions the engine has disowned.
//
// Only the manifest makes this decidable. Without a record of what was written,
// a file the engine does not ship is indistinguishable from one the user added,
// and deleting it would be a guess. Files with no recorded hash are therefore
// never considered, which also means an instance predating manifests loses
// nothing.
//
// Entries already gone from disk are returned as Missing so the caller can drop
// the stale bookkeeping without touching the filesystem.
func PlanRemovals(root string, engine map[string][]byte, m *Manifest) []Change {
	paths := make([]string, 0)
	for p := range m.Files {
		if _, still := engine[p]; !still {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	out := make([]Change, 0, len(paths))
	for _, p := range paths {
		c := Change{Path: p}
		// The manifest is a file on disk, and this decides what gets deleted.
		// A path that escapes the instance is corruption, not an instruction.
		if !fs.ValidPath(p) || strings.Contains(p, "..") {
			continue
		}
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		switch {
		case err != nil:
			c.Status = Missing
		case Sum(got) == m.Files[p]:
			c.Status = Withdrawn
		default:
			c.Status = WithdrawnEdited
		}
		out = append(out, c)
	}
	return out
}

// Plan classifies every engine-owned file, sorted by path so output and tests
// are stable.
//
// The Unknown case is the one that matters in practice: an instance scaffolded
// before manifests existed has no record of anything, so a file that differs
// cannot be attributed. Refusing to touch it is the only safe answer — an
// upgrade that guesses wrong here destroys work with no way back.
func Plan(root string, engine map[string][]byte, m *Manifest) []Change {
	paths := make([]string, 0, len(engine))
	for p := range engine {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	out := make([]Change, 0, len(paths))
	for _, p := range paths {
		want := engine[p]
		c := Change{Path: p, Body: want}
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		switch {
		case err != nil:
			c.Status = Missing
		case Sum(got) == Sum(want):
			c.Status = Current
		case m.Files[p] == "":
			c.Status = Unknown
		case m.Files[p] == Sum(got):
			c.Status = Stale
		default:
			c.Status = Modified
		}
		out = append(out, c)
	}
	return out
}
