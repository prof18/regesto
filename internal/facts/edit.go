package facts

import (
	"fmt"
	"os"
	"strings"
)

// SetFields rewrites named frontmatter keys in place, inserting any that are
// absent just before the closing `---`.
//
// It edits lines rather than re-serialising the parsed struct on purpose: a fact
// file is hand-written as often as it is generated, and round-tripping would
// silently drop key order, comments, blank lines and any field this parser does
// not know about. Lint must be able to flip one field without disturbing the
// rest of somebody's file.
//
// The body is never touched — SCHEMA.md forbids editing a claim's prose to
// "fix" it.
func SetFields(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fmt.Errorf("%s: no frontmatter", path)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return fmt.Errorf("%s: unterminated frontmatter", path)
	}

	remaining := make(map[string]string, len(updates))
	for k, v := range updates {
		remaining[k] = v
	}

	out := make([]string, 0, len(lines)+len(updates))
	out = append(out, lines[0])
	for i := 1; i < end; i++ {
		line := lines[i]
		key, _, ok := strings.Cut(line, ":")
		if ok {
			k := strings.TrimSpace(key)
			if v, want := remaining[k]; want {
				// Preserve the original alignment padding, which several
				// fields use to line `created:` up under `modified:`.
				pad := ""
				if rest, found := strings.CutPrefix(line, k+":"); found {
					pad = rest[:len(rest)-len(strings.TrimLeft(rest, " "))]
				}
				if pad == "" {
					pad = " "
				}
				out = append(out, k+":"+pad+v)
				delete(remaining, k)
				continue
			}
		}
		out = append(out, line)
	}
	// Anything not already present is inserted before the closing marker, in a
	// stable order so repeated runs produce identical files.
	for _, k := range []string{"schema_version", "id", "title", "type", "scope",
		"subject", "relation", "topics", "status", "supersedes", "source",
		"created", "modified", "review_after"} {
		if v, ok := remaining[k]; ok {
			out = append(out, k+": "+v)
			delete(remaining, k)
		}
	}
	for k, v := range remaining {
		out = append(out, k+": "+v)
	}
	out = append(out, lines[end:]...)

	return writeAtomic(path, []byte(strings.Join(out, "\n")))
}

// writeAtomic writes via a temporary file in the same directory and renames.
// Hermes is always-on and lint runs hourly, so there is no quiet window
// (DECISION §8) — a reader must never observe a half-written fact.
func writeAtomic(path string, data []byte) error {
	dir := path[:strings.LastIndex(path, "/")+1]
	tmp, err := os.CreateTemp(dir, ".regesto-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
