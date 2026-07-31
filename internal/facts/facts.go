// Package facts reads atomic fact files from knowledge/facts/ and parses
// their frontmatter per SCHEMA.md ("A fact"). One claim per file.
package facts

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	StatusActive     = "active"
	StatusProposed   = "proposed"
	StatusSuperseded = "superseded"
)

// Fact is one claim: SCHEMA.md frontmatter plus the body text.
type Fact struct {
	SchemaVersion string
	ID            string
	Title         string
	Type          string
	Scope         string
	Subject       string
	Relation      string
	Topics        []string
	Status        string
	Supersedes    string
	Source        string
	Created       string
	Modified      string
	ReviewAfter   string
	Body          string
	// RelPath is the file's path relative to the KB root, e.g.
	// knowledge/facts/projects/aurora/dec-http-port-8080.md.
	RelPath string
}

// DefaultConflictPattern matches what Syncthing inserts into a filename when
// two machines changed the same file:
// name.sync-conflict-<date>-<time>-<id>.md.
const DefaultConflictPattern = `\.sync-conflict-[^.]*`

// conflictPattern is what identifies a sync client's conflict copy, and what is
// cut out of the name to get back to the original.
//
// A pattern rather than a literal marker, because clients differ in shape, not
// just in wording: Syncthing appends before the extension, while others wrap the
// insertion in brackets mid-name. Removing a matched span handles both; taking
// the text before a marker only handles the first.
var conflictPattern = regexp.MustCompile(DefaultConflictPattern)

// SetConflictPattern points conflict detection at another sync client's naming.
// Process-wide and set once from the instance config, because the alternative is
// threading it through every function that loads a fact — and a fact loader that
// disagrees with the conflict finder would parse conflict copies as real facts.
func SetConflictPattern(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return fmt.Errorf("conflict_pattern %q is not a valid regular expression: %w", expr, err)
	}
	conflictPattern = re
	return nil
}

// IsConflict reports whether a filename is a sync client's conflict copy.
func IsConflict(name string) bool { return conflictPattern.MatchString(name) }

// LoadAll walks knowledge/facts/ under kbRoot and parses every .md file.
// Files that fail to parse abort the load: a malformed fact is a schema
// violation to surface, not skip silently.
//
// Conflict copies are skipped. They hold the same id as the file they conflict
// with, so loading them would surface two claims with one identity through
// search, the index and the SessionStart hook — an unresolved conflict must not
// reach a session's context. Lint finds them with FindConflicts and resolves
// them explicitly.
func LoadAll(kbRoot string) ([]Fact, error) {
	factsDir := filepath.Join(kbRoot, "knowledge", "facts")
	var out []Fact
	err := filepath.WalkDir(factsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") || IsConflict(d.Name()) {
			return nil
		}
		f, err := ParseFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(kbRoot, path)
		if err != nil {
			return err
		}
		f.RelPath = filepath.ToSlash(rel)
		out = append(out, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}

// ParseFile reads one fact file: `---` frontmatter block, then the body.
func ParseFile(path string) (Fact, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Fact{}, err
	}
	return Parse(raw, path)
}

// Parse reads a fact from bytes. label only appears in error messages, so a
// candidate that has not been written anywhere yet can still be checked.
func Parse(raw []byte, label string) (Fact, error) {
	path := label
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Fact{}, fmt.Errorf("%s: no frontmatter (file must start with ---)", path)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return Fact{}, fmt.Errorf("%s: unterminated frontmatter", path)
	}

	f := Fact{}
	for i := 1; i < end; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Fact{}, fmt.Errorf("%s: bad frontmatter line %d: %q", path, i+1, line)
		}
		key = strings.TrimSpace(key)
		value = unquote(strings.TrimSpace(value))
		switch key {
		case "schema_version":
			f.SchemaVersion = value
		case "id":
			f.ID = value
		case "title":
			f.Title = value
		case "type":
			f.Type = value
		case "scope":
			f.Scope = value
		case "subject":
			f.Subject = value
		case "relation":
			f.Relation = value
		case "topics":
			f.Topics = parseInlineList(value)
		case "status":
			f.Status = value
		case "supersedes":
			f.Supersedes = value
		case "source":
			f.Source = value
		case "created":
			f.Created = value
		case "modified":
			f.Modified = value
		case "review_after":
			f.ReviewAfter = value
		default:
			// Unknown keys are tolerated on read; lint (Phase 2) is the
			// place that rejects them.
		}
	}
	f.Body = strings.TrimSpace(strings.Join(lines[end+1:], "\n"))

	for field, v := range map[string]string{
		"id": f.ID, "title": f.Title, "type": f.Type, "scope": f.Scope,
		"subject": f.Subject, "relation": f.Relation, "status": f.Status,
	} {
		if v == "" {
			return Fact{}, fmt.Errorf("%s: required frontmatter field %q missing", path, field)
		}
	}
	return f, nil
}

// ProjectName returns <name> for scope `project:<name>`, "" for global.
func (f Fact) ProjectName() string {
	if name, ok := strings.CutPrefix(f.Scope, "project:"); ok {
		return name
	}
	return ""
}

// unquote strips one layer of matching surrounding quotes, as YAML does. A
// value must be quoted when it contains a colon — `title: "Reorder UX: ..."` —
// and the quotes are syntax, not content.
func unquote(s string) string {
	if len(s) >= 2 {
		q := s[0]
		if (q == '"' || q == '\'') && s[len(s)-1] == q {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseInlineList parses `[a, b, c]` (SCHEMA.md's topics form).
func parseInlineList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var out []string
	for _, item := range strings.Split(value, ",") {
		item = strings.Trim(strings.TrimSpace(item), `"'`)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
