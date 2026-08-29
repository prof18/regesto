package install

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/adapters"
)

type skillSource struct {
	name string
	dir  string
	fsys fs.FS
}

func planSkills(p *Plan, agents []adapters.Agent) error {
	sourceRoot := filepath.Join(p.KBRoot, "adapters", "skills")
	var sourceFS fs.FS = os.DirFS(sourceRoot)
	sourceDir := "."
	entries, err := fs.ReadDir(sourceFS, sourceDir)
	if os.IsNotExist(err) {
		sourceFS = regesto.Adapters
		sourceDir = "adapters/skills"
		entries, err = fs.ReadDir(sourceFS, sourceDir)
	}
	if err != nil {
		return fmt.Errorf("read shipped skills: %w", err)
	}
	var sources []skillSource
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			sources = append(sources, skillSource{name: entry.Name(), dir: filepath.ToSlash(filepath.Join(sourceDir, entry.Name())), fsys: sourceFS})
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].name < sources[j].name })
	stage := filepath.Join(p.KBRoot, ".state", "skills")
	canonicalRoot, err := CanonicalTarget(p.KBRoot)
	if err != nil {
		return err
	}
	canonicalStage, err := CanonicalTarget(stage)
	if err != nil {
		return err
	}
	if !within(canonicalRoot, canonicalStage) {
		return fmt.Errorf("refuse rendered skill stage outside the knowledge base: %s resolves to %s", stage, canonicalStage)
	}
	var skillOwners []string
	for _, agent := range agents {
		if len(agent.SkillsDirs) > 0 {
			skillOwners = append(skillOwners, agent.Name)
		}
	}
	skillOwners = sortedUnique(skillOwners)
	for _, source := range sources {
		expected := map[string]bool{}
		sourceItemsStart := len(p.Items)
		err := fs.WalkDir(source.fsys, source.dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				return nil
			}
			rel, err := filepath.Rel(source.dir, path)
			if err != nil {
				return err
			}
			expected[filepath.Clean(rel)] = true
			body, err := fs.ReadFile(source.fsys, path)
			if err != nil {
				return err
			}
			if strings.EqualFold(filepath.Ext(path), ".md") {
				body = render(body, p.KBRoot)
			}
			target := filepath.Join(stage, source.name, rel)
			canonical, err := CanonicalTarget(target)
			if err != nil {
				return err
			}
			if !within(canonicalStage, canonical) {
				return fmt.Errorf("refuse rendered skill outside stage: %s resolves to %s", target, canonical)
			}
			item := Item{
				ID:              "skill-render:" + source.name + ":" + filepath.ToSlash(rel),
				Kind:            "skill-render",
				Owners:          skillOwners,
				DeclaredTargets: []string{target},
				CanonicalTarget: canonical,
				IntendedState:   "rendered from " + filepath.ToSlash(filepath.Join("adapters", "skills", source.name, rel)),
				desired:         body,
				mode:            0o644,
			}
			current, err := os.ReadFile(canonical)
			switch {
			case err == nil && bytes.Equal(current, body):
				item.Action = "current"
				item.CurrentState = "rendered file current"
				item.BackupAction = "none"
				item.DryRun = "leave rendered skill file unchanged"
			case err == nil:
				item.Action = "update"
				item.CurrentState = "rendered file stale"
				item.before = current
				item.BackupAction = "none (Regesto-owned stage)"
				item.DryRun = "refresh Regesto-owned rendered skill file"
			case os.IsNotExist(err):
				item.Action = "create"
				item.CurrentState = "rendered file missing"
				item.BackupAction = "none (target does not exist)"
				item.DryRun = "create Regesto-owned rendered skill file"
			default:
				return err
			}
			p.Items = append(p.Items, item)
			return nil
		})
		if err != nil {
			return err
		}
		markerTarget := filepath.Join(stage, source.name, ".regesto-owned")
		markerCanonical, err := CanonicalTarget(markerTarget)
		if err != nil {
			return err
		}
		if !within(canonicalStage, markerCanonical) {
			return fmt.Errorf("refuse rendered skill marker outside stage: %s resolves to %s", markerTarget, markerCanonical)
		}
		marker := stageMarker(canonicalRoot, source.name)
		markerOwned := false
		item := Item{
			ID:              "skill-render:" + source.name + ":.regesto-owned",
			Kind:            "skill-render",
			Owners:          skillOwners,
			DeclaredTargets: []string{markerTarget},
			CanonicalTarget: markerCanonical,
			IntendedState:   "Regesto ownership marker for generated skill stage",
			desired:         marker,
			mode:            0o600,
		}
		current, readErr := os.ReadFile(markerCanonical)
		switch {
		case readErr == nil && bytes.Equal(current, marker):
			markerOwned = true
			item.Action, item.CurrentState = "current", "ownership marker current"
			item.BackupAction, item.DryRun = "none", "leave ownership marker unchanged"
		case readErr == nil:
			return fmt.Errorf("refuse rendered skill stage with mismatched ownership marker: %s", markerCanonical)
		case os.IsNotExist(readErr):
			adoptable, err := adoptableStage(filepath.Join(stage, source.name), expected)
			if err != nil {
				return err
			}
			if !adoptable {
				return fmt.Errorf("refuse unowned rendered skill directory with unexpected files: %s", filepath.Join(stage, source.name))
			}
			item.Action, item.CurrentState = "create", "ownership marker missing"
			item.BackupAction, item.DryRun = "none (Regesto-owned stage)", "create ownership marker for generated skill stage"
		default:
			return readErr
		}
		for i := sourceItemsStart; i < len(p.Items); i++ {
			p.Items[i].ownership = marker
			p.Items[i].ownershipTarget = markerCanonical
		}
		p.Items = append(p.Items, item)
		if markerOwned {
			if err := planStaleSkillFiles(p, source.name, filepath.Join(stage, source.name), canonicalStage, canonicalRoot, expected, skillOwners); err != nil {
				return err
			}
		}
	}
	if err := planStaleRenders(p, stage, canonicalStage, sources, skillOwners); err != nil {
		return err
	}
	return planSkillTargets(p, agents, sources, stage, canonicalStage)
}

func planStaleRenders(p *Plan, stage, canonicalStage string, sources []skillSource, owners []string) error {
	canonicalRoot, err := CanonicalTarget(p.KBRoot)
	if err != nil {
		return err
	}
	shipped := map[string]bool{}
	for _, source := range sources {
		shipped[source.name] = true
	}
	entries, err := os.ReadDir(stage)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || shipped[entry.Name()] {
			continue
		}
		target := filepath.Join(stage, entry.Name())
		canonical, err := CanonicalTarget(target)
		if err != nil {
			return err
		}
		if !within(canonicalStage, canonical) {
			return fmt.Errorf("refuse stale rendered skill outside stage: %s resolves to %s", target, canonical)
		}
		markerPath := filepath.Join(canonical, ".regesto-owned")
		marker, markerErr := os.ReadFile(markerPath)
		if markerErr != nil || !bytes.Equal(marker, stageMarker(canonicalRoot, entry.Name())) {
			p.Items = append(p.Items, Item{
				ID:              "skill-render-preserve:" + entry.Name(),
				Kind:            "skill-render",
				Action:          "skip",
				Owners:          []string{},
				DeclaredTargets: []string{target},
				CanonicalTarget: canonical,
				CurrentState:    "unshipped stage directory without a matching Regesto ownership marker",
				IntendedState:   "foreign directory preserved",
				BackupAction:    "none",
				DryRun:          "leave unowned stage directory unchanged",
			})
			continue
		}
		digest, err := treeDigest(canonical)
		if err != nil {
			return err
		}
		p.Items = append(p.Items, Item{
			ID:              "skill-render-prune:" + entry.Name(),
			Kind:            "skill-render-prune",
			Action:          "remove",
			Owners:          owners,
			DeclaredTargets: []string{target},
			CanonicalTarget: canonical,
			CurrentState:    "Regesto-owned rendered skill has no shipped source",
			IntendedState:   "absent",
			BackupAction:    "none (Regesto-owned generated stage)",
			DryRun:          "remove stale Regesto-owned rendered skill directory",
			before:          digest,
			ownership:       stageMarker(canonicalRoot, entry.Name()),
		})
	}
	return nil
}

func treeDigest(root string) ([]byte, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%s\x00", entry.Type().String(), filepath.ToSlash(rel))
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			hash.Write([]byte(target))
		case !entry.IsDir():
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hash.Write(body)
		}
		hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

func stageMarker(canonicalRoot, skill string) []byte {
	return []byte("regesto-generated-skill-stage-v1\nkb_root=" + canonicalRoot + "\nskill=" + skill + "\n")
}

func adoptableStage(dir string, expected map[string]bool) (bool, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	adoptable := true
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.Clean(rel)
		if entry.IsDir() {
			prefix := rel + string(filepath.Separator)
			for candidate := range expected {
				candidate = filepath.Clean(candidate)
				if candidate == rel || strings.HasPrefix(candidate, prefix) {
					return nil
				}
			}
			adoptable = false
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeType != 0 || !expected[rel] {
			adoptable = false
		}
		return nil
	})
	return adoptable, err
}

func planStaleSkillFiles(p *Plan, skill, declaredDir, canonicalStage, canonicalRoot string, expected map[string]bool, owners []string) error {
	canonicalDir, err := CanonicalTarget(declaredDir)
	if err != nil {
		return err
	}
	info, err := os.Stat(canonicalDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || !within(canonicalStage, canonicalDir) {
		return fmt.Errorf("refuse invalid rendered skill directory %s", canonicalDir)
	}
	return filepath.WalkDir(canonicalDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(canonicalDir, path)
		if err != nil {
			return err
		}
		if rel == ".regesto-owned" || expected[filepath.Clean(rel)] {
			return nil
		}
		canonical, err := CanonicalTarget(path)
		if err != nil {
			return err
		}
		if !within(canonicalDir, canonical) {
			return fmt.Errorf("refuse rendered skill file outside stage: %s resolves to %s", path, canonical)
		}
		if entry.Type()&os.ModeType != 0 {
			return fmt.Errorf("refuse to prune non-regular generated skill entry %s", path)
		}
		body, err := os.ReadFile(canonical)
		if err != nil {
			return err
		}
		p.Items = append(p.Items, Item{
			ID:              "skill-render-file-prune:" + skill + ":" + filepath.ToSlash(rel),
			Kind:            "skill-render-file",
			Action:          "remove",
			Owners:          owners,
			DeclaredTargets: []string{filepath.Join(declaredDir, rel)},
			CanonicalTarget: canonical,
			CurrentState:    "generated skill file has no shipped source",
			IntendedState:   "absent",
			BackupAction:    "none (Regesto-owned generated stage)",
			DryRun:          "remove stale generated skill file",
			before:          body,
			ownership:       stageMarker(canonicalRoot, skill),
		})
		return nil
	})
}

func planSkillTargets(p *Plan, agents []adapters.Agent, sources []skillSource, stage, canonicalStage string) error {
	type dirGroup struct {
		canonical string
		declared  []string
		owners    []string
	}
	groups := map[string]*dirGroup{}
	for _, agent := range agents {
		for _, declared := range agent.SkillsDirs {
			canonical, err := CanonicalTarget(declared)
			if err != nil {
				return err
			}
			g := groups[canonical]
			if g == nil {
				g = &dirGroup{canonical: canonical}
				groups[canonical] = g
			}
			g.declared = append(g.declared, declared)
			g.owners = append(g.owners, agent.Name)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		g := groups[key]
		g.declared, g.owners = sortedUnique(g.declared), sortedUnique(g.owners)
		info, err := os.Stat(key)
		if err == nil && !info.IsDir() {
			return fmt.Errorf("skills target %s is not a directory", key)
		}
		if os.IsNotExist(err) {
			p.Items = append(p.Items, Item{
				ID:              "skills-dir:" + key,
				Kind:            "skills-directory",
				Action:          "create",
				Owners:          g.owners,
				DeclaredTargets: g.declared,
				CanonicalTarget: key,
				CurrentState:    "missing",
				IntendedState:   "skills directory present",
				BackupAction:    "none (target does not exist)",
				DryRun:          "create the shared skills directory",
				mode:            0o755,
			})
		} else if err != nil {
			return err
		}
		if err := planOwnedDanglingLinks(p, g.owners, g.declared, key, sources, stage, canonicalStage); err != nil {
			return err
		}
		for _, source := range sources {
			dest := filepath.Join(key, source.name)
			want := filepath.Join(stage, source.name)
			item, err := planSkillLink(g.owners, g.declared, dest, want, p.KBRoot)
			if err != nil {
				return err
			}
			p.Items = append(p.Items, item)
		}
	}
	return nil
}

func planOwnedDanglingLinks(p *Plan, owners, declaredDirs []string, canonicalDir string, sources []skillSource, stage, canonicalStage string) error {
	canonicalRoot, err := CanonicalTarget(p.KBRoot)
	if err != nil {
		return err
	}
	shipped := map[string]bool{}
	for _, source := range sources {
		shipped[source.name] = true
	}
	entries, err := os.ReadDir(canonicalDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if shipped[entry.Name()] || entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		dest := filepath.Join(canonicalDir, entry.Name())
		raw, err := os.Readlink(dest)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(raw) {
			raw = filepath.Join(canonicalDir, raw)
		}
		raw = filepath.Clean(raw)
		legacyOwned := strings.HasPrefix(raw, filepath.Join(p.KBRoot, "adapters", "skills")+string(filepath.Separator))
		stageOwned := false
		for _, stageRoot := range []string{filepath.Clean(stage), canonicalStage} {
			prefix := stageRoot + string(filepath.Separator)
			if !strings.HasPrefix(raw, prefix) {
				continue
			}
			rel := strings.TrimPrefix(raw, prefix)
			skillName := strings.Split(filepath.ToSlash(rel), "/")[0]
			marker, err := os.ReadFile(filepath.Join(stageRoot, skillName, ".regesto-owned"))
			if err == nil && bytes.Equal(marker, stageMarker(canonicalRoot, skillName)) {
				stageOwned = true
			}
		}
		owned := legacyOwned || stageOwned
		if !owned {
			continue
		}
		declared := make([]string, 0, len(declaredDirs))
		for _, dir := range declaredDirs {
			declared = append(declared, filepath.Join(dir, entry.Name()))
		}
		p.Items = append(p.Items, Item{
			ID:              "skill-link-prune:" + dest,
			Kind:            "skill-link",
			Action:          "remove",
			Owners:          owners,
			DeclaredTargets: sortedUnique(declared),
			CanonicalTarget: dest,
			CurrentState:    "dangling Regesto-owned skill link to " + raw,
			IntendedState:   "absent",
			BackupAction:    "none (Regesto-owned symlink)",
			DryRun:          "remove dangling Regesto-owned skill link",
			before:          []byte(raw),
		})
	}
	return nil
}

func planSkillLink(owners, declaredDirs []string, dest, want, root string) (Item, error) {
	canonical, err := canonicalLinkPath(dest)
	if err != nil {
		return Item{}, err
	}
	declared := make([]string, 0, len(declaredDirs))
	for _, dir := range declaredDirs {
		declared = append(declared, filepath.Join(dir, filepath.Base(dest)))
	}
	item := Item{
		ID:              "skill-link:" + canonical,
		Kind:            "skill-link",
		Owners:          owners,
		DeclaredTargets: sortedUnique(declared),
		CanonicalTarget: canonical,
		IntendedState:   "symbolic link to " + want,
		BackupAction:    "none",
		linkTarget:      want,
	}
	info, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		item.Action = "create"
		item.CurrentState = "missing"
		item.DryRun = "create skill link"
		return item, nil
	}
	if err != nil {
		return Item{}, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		item.Action = "skip"
		item.CurrentState = "foreign non-symlink entry preserved"
		item.DryRun = "leave foreign skill entry unchanged"
		return item, nil
	}
	raw, err := os.Readlink(dest)
	if err != nil {
		return Item{}, err
	}
	resolved, resolveErr := filepath.EvalSymlinks(dest)
	wantResolved, wantErr := filepath.EvalSymlinks(want)
	if resolveErr == nil && wantErr == nil && resolved == wantResolved {
		item.Action = "current"
		item.CurrentState = "owned skill link current"
		item.DryRun = "leave skill link unchanged"
		return item, nil
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(filepath.Dir(dest), raw)
	}
	raw = filepath.Clean(raw)
	legacyTarget := filepath.Join(root, "adapters", "skills", filepath.Base(dest))
	if raw == legacyTarget || stageSkillLinkOwned(root, raw, filepath.Base(dest)) {
		item.Action = "replace"
		item.CurrentState = "legacy or stale Regesto-owned skill link"
		item.DryRun = "replace Regesto-owned skill link"
		item.before = []byte(raw)
		return item, nil
	}
	item.Action = "skip"
	item.CurrentState = "foreign symlink to " + raw + " preserved"
	item.DryRun = "leave foreign skill link unchanged"
	return item, nil
}

func stageSkillLinkOwned(root, raw, skill string) bool {
	canonicalRoot, err := CanonicalTarget(root)
	if err != nil {
		return false
	}
	canonicalStage, err := CanonicalTarget(filepath.Join(root, ".state", "skills"))
	if err != nil {
		return false
	}
	canonicalTarget, err := CanonicalTarget(raw)
	if err != nil || filepath.Dir(canonicalTarget) != canonicalStage || filepath.Base(canonicalTarget) != skill {
		return false
	}
	marker, err := os.ReadFile(filepath.Join(canonicalTarget, ".regesto-owned"))
	return err == nil && bytes.Equal(marker, stageMarker(canonicalRoot, skill))
}

// canonicalLinkPath resolves the containing directory but deliberately does
// not follow the final entry: the symlink itself is the object install owns.
func canonicalLinkPath(path string) (string, error) {
	parent, err := CanonicalTarget(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}
