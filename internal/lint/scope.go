package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
)

// ScopeFix is one fact filed under a non-canonical project name.
type ScopeFix struct {
	ID      string
	From    string // path relative to the KB root
	To      string
	Scope   string // the canonical scope it should carry
	Message string
	Blocked string // non-empty when the move cannot be made safely
}

// CanonicaliseScopes finds facts filed under a project alias and moves them to
// the canonical name (PLAN 0.2).
//
// `scope` decides both the file's location and what a scoped search returns, so
// one project spelled two ways splits its knowledge in half silently: a session
// in that repo sees only the half matching the name the hook resolved. The
// [projects] table already maps alias to canonical for repositories; this applies
// the same map to facts.
//
// Only names explicitly listed as aliases are touched. Similar-looking names are
// left alone — `beacon` and `beacon-mobile` are two real projects, and
// guessing by resemblance would merge them.
func CanonicaliseScopes(kbRoot string, cfg *config.Config, all []facts.Fact, apply bool, now time.Time) ([]ScopeFix, error) {
	var out []ScopeFix

	// Ids already in use, so a move cannot silently overwrite a fact.
	byID := map[string]string{}
	for _, f := range all {
		byID[f.ID] = f.RelPath
	}

	for _, f := range all {
		project := f.ProjectName()
		if project == "" {
			continue
		}
		canonical, ok := cfg.Projects[project]
		if !ok || canonical == project {
			continue
		}

		fix := ScopeFix{
			ID:    f.ID,
			From:  f.RelPath,
			To:    "knowledge/facts/projects/" + canonical + "/" + f.ID + ".md",
			Scope: "project:" + canonical,
			Message: fmt.Sprintf("filed under project:%s, which config.toml maps to %s",
				project, canonical),
		}

		if existing, clash := byID[f.ID]; clash && existing != f.RelPath {
			fix.Blocked = "another fact already uses this id: " + existing
			out = append(out, fix)
			continue
		}
		if _, err := os.Stat(filepath.Join(kbRoot, fix.To)); err == nil {
			fix.Blocked = "a file already exists at the destination"
			out = append(out, fix)
			continue
		}

		if apply {
			dest := filepath.Join(kbRoot, fix.To)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return out, err
			}
			// Rewrite the scope before moving, so a failure between the two
			// leaves a file whose scope and location still agree.
			src := filepath.Join(kbRoot, f.RelPath)
			if err := facts.SetFields(src, map[string]string{
				"scope":    fix.Scope,
				"modified": now.UTC().Format(time.RFC3339),
			}); err != nil {
				return out, err
			}
			if err := os.Rename(src, dest); err != nil {
				return out, err
			}
			// Leave no empty alias directory behind to invite the mistake again.
			_ = os.Remove(filepath.Dir(src))
		}
		out = append(out, fix)
	}
	return out, nil
}

// KnownProjects lists the canonical project names in use, for prompts that must
// steer a model towards reusing one rather than inventing a spelling.
func KnownProjects(cfg *config.Config, all []facts.Fact) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	// Canonical targets from config first: these are correct even before any
	// fact uses them.
	for alias, canonical := range cfg.Projects {
		if canonical != alias {
			add(canonical)
		}
	}
	for _, f := range all {
		if p := f.ProjectName(); p != "" {
			if canonical, ok := cfg.Projects[p]; ok {
				add(canonical)
				continue
			}
			add(p)
		}
	}
	return out
}
