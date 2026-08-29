package install

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/adapters"
)

var portableSkillName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var safeIntegrationStageName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type skillSource struct {
	name  string
	files map[string][]byte
}

type skillVariant struct {
	SchemaVersion         int      `json:"schema_version"`
	ID                    string   `json:"id"`
	Mode                  string   `json:"mode"`
	RequiresHookProtocols []string `json:"requires_hook_protocols"`
}

type integrationRender struct {
	agent          adapters.Agent
	variant        string
	stage          string
	canonicalStage string
	sources        []skillSource
}

func planSkills(p *Plan, agents []adapters.Agent, sourceRoot string) error {
	portable, err := loadPortableSkills(sourceRoot)
	if err != nil {
		return err
	}
	canonicalRoot, err := CanonicalTarget(p.KBRoot)
	if err != nil {
		return err
	}

	var renders []integrationRender
	seenIntegrations := map[string]bool{}
	for _, agent := range agents {
		if len(agent.SkillsDirs) == 0 {
			continue
		}
		if seenIntegrations[agent.Name] {
			continue
		}
		seenIntegrations[agent.Name] = true
		variant := agent.SkillsVariant
		if variant == "" {
			variant = "portable"
		}
		sources, err := renderSkillVariant(p.KBRoot, sourceRoot, agent, variant, portable)
		if err != nil {
			return err
		}
		stage := filepath.Join(p.KBRoot, ".state", "integrations", integrationStageName(agent.Name), "skills")
		canonicalStage, err := CanonicalTarget(stage)
		if err != nil {
			return err
		}
		if !within(canonicalRoot, canonicalStage) {
			return fmt.Errorf("refuse rendered integration skills outside the knowledge base: %s resolves to %s", stage, canonicalStage)
		}
		rendered := integrationRender{agent: agent, variant: variant, stage: stage, canonicalStage: canonicalStage, sources: sources}
		if err := planIntegrationRender(p, canonicalRoot, rendered); err != nil {
			return err
		}
		renders = append(renders, rendered)
	}
	if err := planSkillTargets(p, renders); err != nil {
		return err
	}

	// Migrate the pre-variant shared render stage after new per-integration
	// trees and replacement links have been fully described in the same plan.
	legacyStage := filepath.Join(p.KBRoot, ".state", "skills")
	legacyCanonical, err := CanonicalTarget(legacyStage)
	if err != nil {
		return err
	}
	if !within(canonicalRoot, legacyCanonical) {
		return fmt.Errorf("refuse legacy rendered skill stage outside the knowledge base: %s resolves to %s", legacyStage, legacyCanonical)
	}
	owners := make([]string, 0, len(renders))
	for _, rendered := range renders {
		owners = append(owners, rendered.agent.Name)
	}
	return planStaleSkillDirs(p, legacyStage, legacyCanonical, nil, sortedUnique(owners), func(skill string) []byte {
		return stageMarker(canonicalRoot, skill)
	})
}

func loadPortableSkills(sourceRoot string) ([]skillSource, error) {
	root := filepath.Join(sourceRoot, "adapters", "skills")
	var sourceFS fs.FS = os.DirFS(root)
	dir := "."
	instanceSource := true
	entries, err := fs.ReadDir(sourceFS, dir)
	if os.IsNotExist(err) {
		sourceFS, dir = regesto.Adapters, "adapters/skills"
		instanceSource = false
		entries, err = fs.ReadDir(sourceFS, dir)
	}
	if err != nil {
		return nil, fmt.Errorf("read shipped skills: %w", err)
	}
	var sources []skillSource
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		source := skillSource{name: entry.Name(), files: map[string][]byte{}}
		sourceDir := filepath.ToSlash(filepath.Join(dir, entry.Name()))
		err := fs.WalkDir(sourceFS, sourceDir, func(path string, item fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == sourceDir {
				return nil
			}
			if strings.HasPrefix(item.Name(), ".") {
				if item.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if item.IsDir() {
				return nil
			}
			if item.Type()&os.ModeType != 0 {
				return fmt.Errorf("portable skill %s contains non-regular entry %s", source.name, path)
			}
			rel, err := filepath.Rel(sourceDir, path)
			if err != nil {
				return err
			}
			body, err := fs.ReadFile(sourceFS, path)
			if err != nil {
				return err
			}
			source.files[filepath.Clean(rel)] = body
			return nil
		})
		if err != nil {
			return nil, err
		}
		skill, ok := source.files["SKILL.md"]
		if !ok {
			return nil, fmt.Errorf("portable skill %s has no SKILL.md", source.name)
		}
		if err := validatePortableSkill(source.name, skill); err != nil {
			if instanceSource {
				return nil, fmt.Errorf("instance skill %s is not portable: %w; run `regesto upgrade` and then retry install", source.name, err)
			}
			return nil, fmt.Errorf("portable skill %s: %w", source.name, err)
		}
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].name < sources[j].name })
	return sources, nil
}

func validatePortableSkill(directory string, body []byte) error {
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 4 || lines[0] != "---" {
		return fmt.Errorf("SKILL.md must begin with YAML frontmatter")
	}
	closing := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			closing = i
			break
		}
	}
	if closing < 0 {
		return fmt.Errorf("SKILL.md frontmatter is not closed")
	}
	allowed := map[string]bool{"name": true, "description": true, "license": true, "compatibility": true, "metadata": true}
	values := map[string]string{}
	metadata := map[string]bool{}
	inMetadata := false
	for index := 1; index < closing; index++ {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(line, "\t") {
			return fmt.Errorf("tabs are not supported in portable frontmatter")
		}
		if line[0] == ' ' {
			if !inMetadata || !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") {
				return fmt.Errorf("indented frontmatter is allowed only for two-space metadata entries")
			}
			key, raw, ok := strings.Cut(strings.TrimPrefix(line, "  "), ":")
			if !ok || strings.TrimSpace(key) != key || key == "" || metadata[key] {
				return fmt.Errorf("invalid or duplicate metadata entry %q", line)
			}
			if _, err := parsePortableYAMLScalar(strings.TrimSpace(raw)); err != nil {
				return fmt.Errorf("metadata %q: %w", key, err)
			}
			metadata[key] = true
			continue
		}
		inMetadata = false
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != key || !allowed[key] {
			return fmt.Errorf("unsupported portable frontmatter field in %q", line)
		}
		if _, duplicate := values[key]; duplicate {
			return fmt.Errorf("duplicate frontmatter field %q", key)
		}
		raw := strings.TrimSpace(value)
		if key == "metadata" {
			if raw != "" {
				return fmt.Errorf("metadata must be a nested string mapping")
			}
			inMetadata = true
			values[key] = "mapping"
			continue
		}
		var (
			parsed string
			err    error
		)
		if isPortableYAMLBlockHeader(raw) {
			var next int
			parsed, next, err = parsePortableYAMLBlock(lines, index+1, closing)
			if err == nil {
				index = next - 1
			}
		} else {
			parsed, err = parsePortableYAMLScalar(raw)
		}
		if err != nil {
			return fmt.Errorf("frontmatter %q: %w", key, err)
		}
		values[key] = parsed
	}
	name := values["name"]
	if name != directory || !portableSkillName.MatchString(name) || utf8.RuneCountInString(name) > 64 {
		return fmt.Errorf("name %q must match its directory and the portable naming contract", name)
	}
	description := values["description"]
	if description == "" || utf8.RuneCountInString(description) > 1024 {
		return fmt.Errorf("description must contain 1 to 1024 characters")
	}
	if compatibility, present := values["compatibility"]; present && (compatibility == "" || utf8.RuneCountInString(compatibility) > 500) {
		return fmt.Errorf("compatibility must contain 1 to 500 characters when present")
	}
	return nil
}

func isPortableYAMLBlockHeader(raw string) bool {
	switch raw {
	case ">", ">-", ">+", "|", "|-", "|+":
		return true
	default:
		return false
	}
}

func parsePortableYAMLBlock(lines []string, start, closing int) (string, int, error) {
	end, indent := start, 0
	for end < closing {
		line := lines[end]
		if strings.Contains(line, "\t") {
			return "", end, fmt.Errorf("tabs are not supported in block scalars")
		}
		if strings.TrimSpace(line) == "" {
			end++
			continue
		}
		spaces := len(line) - len(strings.TrimLeft(line, " "))
		if spaces == 0 {
			break
		}
		if indent == 0 || spaces < indent {
			indent = spaces
		}
		end++
	}
	if indent == 0 {
		return "", end, fmt.Errorf("block scalar must contain indented text")
	}
	var content []string
	for _, line := range lines[start:end] {
		if strings.TrimSpace(line) == "" {
			content = append(content, "")
			continue
		}
		if len(line) < indent {
			return "", end, fmt.Errorf("invalid block scalar indentation")
		}
		content = append(content, line[indent:])
	}
	value := strings.TrimSpace(strings.Join(content, "\n"))
	if value == "" || !utf8.ValidString(value) {
		return "", end, fmt.Errorf("value must be a nonempty UTF-8 scalar")
	}
	return value, end, nil
}

func parsePortableYAMLScalar(raw string) (string, error) {
	if raw == "" || !utf8.ValidString(raw) {
		return "", fmt.Errorf("value must be a nonempty UTF-8 scalar")
	}
	if strings.HasPrefix(raw, `"`) {
		value, err := strconv.Unquote(raw)
		if err != nil {
			return "", fmt.Errorf("invalid double-quoted scalar")
		}
		return value, nil
	}
	if strings.HasPrefix(raw, "'") {
		if len(raw) < 2 || !strings.HasSuffix(raw, "'") {
			return "", fmt.Errorf("invalid single-quoted scalar")
		}
		inner := raw[1 : len(raw)-1]
		for i := 0; i < len(inner); i++ {
			if inner[i] != '\'' {
				continue
			}
			if i+1 >= len(inner) || inner[i+1] != '\'' {
				return "", fmt.Errorf("single quotes inside a scalar must be doubled")
			}
			i++
		}
		return strings.ReplaceAll(inner, "''", "'"), nil
	}
	lower := strings.ToLower(raw)
	implicitNonStrings := map[string]bool{
		"null": true, "~": true, "true": true, "false": true, "yes": true, "no": true,
		"on": true, "off": true, ".nan": true, ".inf": true, "+.inf": true, "-.inf": true,
	}
	startsNumeric := (raw[0] >= '0' && raw[0] <= '9') ||
		(len(raw) > 1 && (raw[0] == '+' || raw[0] == '-') && raw[1] >= '0' && raw[1] <= '9') ||
		(len(raw) > 1 && raw[0] == '.' && raw[1] >= '0' && raw[1] <= '9')
	if strings.ContainsAny(raw, "\r\n\t") || strings.Contains(raw, ": ") || strings.Contains(raw, " #") ||
		strings.ContainsAny(raw[:1], "-?:,[]{}#&*!|>@`%") || implicitNonStrings[lower] || startsNumeric {
		return "", fmt.Errorf("unsupported or malformed plain scalar %q", raw)
	}
	return raw, nil
}

func renderSkillVariant(kbRoot, sourceRoot string, agent adapters.Agent, variant string, portable []skillSource) ([]skillSource, error) {
	out := cloneSkillSources(portable)
	if variant == "portable" {
		return renderSkillSources(out, kbRoot), nil
	}
	manifest, overlayFS, overlayDir, err := loadSkillVariant(sourceRoot, variant)
	if err != nil {
		return nil, fmt.Errorf("integration %q: %w", agent.Name, err)
	}
	declared := map[string]bool{}
	for _, hook := range agent.Hooks {
		declared[hook.Protocol] = true
	}
	for _, protocol := range manifest.RequiresHookProtocols {
		if !declared[protocol] {
			return nil, fmt.Errorf("integration %q variant %q requires undeclared hook protocol %q", agent.Name, variant, protocol)
		}
	}
	if manifest.Mode != "append" {
		return nil, fmt.Errorf("integration %q variant %q has unsupported mode %q", agent.Name, variant, manifest.Mode)
	}
	byName := map[string]*skillSource{}
	for i := range out {
		byName[out[i].name] = &out[i]
	}
	skillsDir := filepath.ToSlash(filepath.Join(overlayDir, "skills"))
	err = fs.WalkDir(overlayFS, skillsDir, func(path string, item fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == skillsDir || item.IsDir() {
			return nil
		}
		if item.Type()&os.ModeType != 0 {
			return fmt.Errorf("variant %q contains non-regular overlay %s", variant, path)
		}
		rel, err := filepath.Rel(skillsDir, path)
		if err != nil {
			return err
		}
		parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
		if len(parts) != 2 || filepath.Ext(parts[1]) != ".md" {
			return fmt.Errorf("variant %q overlay %s must extend an existing Markdown skill file", variant, rel)
		}
		source := byName[parts[0]]
		if source == nil {
			return fmt.Errorf("variant %q overlays unknown skill %q", variant, parts[0])
		}
		target := filepath.Clean(filepath.FromSlash(parts[1]))
		base, ok := source.files[target]
		if !ok {
			return fmt.Errorf("variant %q overlays unknown portable file %s", variant, rel)
		}
		appendix, err := fs.ReadFile(overlayFS, path)
		if err != nil {
			return err
		}
		source.files[target] = appendSkillOverlay(base, appendix)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return renderSkillSources(out, kbRoot), nil
}

func loadSkillVariant(sourceRoot, variant string) (skillVariant, fs.FS, string, error) {
	instanceDir := filepath.Join(sourceRoot, "adapters", "variants", variant)
	var sourceFS fs.FS = os.DirFS(instanceDir)
	dir := "."
	body, err := fs.ReadFile(sourceFS, "variant.json")
	if os.IsNotExist(err) {
		sourceFS, dir = regesto.Adapters, filepath.ToSlash(filepath.Join("adapters", "variants", variant))
		body, err = fs.ReadFile(sourceFS, filepath.ToSlash(filepath.Join(dir, "variant.json")))
	}
	if err != nil {
		return skillVariant{}, nil, "", fmt.Errorf("unknown skills variant %q", variant)
	}
	if err := validateUniqueJSON(body); err != nil {
		return skillVariant{}, nil, "", fmt.Errorf("variant %q: %w", variant, err)
	}
	var manifest skillVariant
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, nil, "", fmt.Errorf("variant %q: invalid manifest: %w", variant, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return manifest, nil, "", fmt.Errorf("variant %q: trailing JSON value", variant)
	}
	if manifest.SchemaVersion != 1 || manifest.ID != variant {
		return manifest, nil, "", fmt.Errorf("variant %q manifest identity/schema mismatch", variant)
	}
	return manifest, sourceFS, dir, nil
}

func cloneSkillSources(in []skillSource) []skillSource {
	out := make([]skillSource, len(in))
	for i, source := range in {
		out[i] = skillSource{name: source.name, files: make(map[string][]byte, len(source.files))}
		for path, body := range source.files {
			out[i].files[path] = append([]byte(nil), body...)
		}
	}
	return out
}

func renderSkillSources(sources []skillSource, root string) []skillSource {
	for i := range sources {
		for path, body := range sources[i].files {
			if strings.EqualFold(filepath.Ext(path), ".md") {
				sources[i].files[path] = render(body, root)
			}
		}
	}
	return sources
}

func appendSkillOverlay(base, appendix []byte) []byte {
	return []byte(strings.TrimRight(string(base), "\r\n") + "\n\n" + strings.TrimSpace(string(appendix)) + "\n")
}

func integrationStageName(id string) string {
	if safeIntegrationStageName.MatchString(id) {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "legacy-" + hex.EncodeToString(sum[:8])
}

func planIntegrationRender(p *Plan, canonicalRoot string, rendered integrationRender) error {
	for _, source := range rendered.sources {
		if err := planRenderedSkill(p, canonicalRoot, rendered, source); err != nil {
			return err
		}
	}
	return planStaleSkillDirs(p, rendered.stage, rendered.canonicalStage, rendered.sources, []string{rendered.agent.Name}, func(skill string) []byte {
		return stageMarker(canonicalRoot, integrationStageName(rendered.agent.Name), skill)
	})
}

func planRenderedSkill(p *Plan, canonicalRoot string, rendered integrationRender, source skillSource) error {
	dir := filepath.Join(rendered.stage, source.name)
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse symlinked rendered skill directory: %s", dir)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	markerTarget := filepath.Join(dir, ".regesto-owned")
	markerCanonical, err := CanonicalTarget(markerTarget)
	if err != nil {
		return err
	}
	if !within(rendered.canonicalStage, markerCanonical) {
		return fmt.Errorf("refuse rendered skill marker outside integration stage: %s", markerCanonical)
	}
	marker := stageMarker(canonicalRoot, integrationStageName(rendered.agent.Name), source.name)
	expected := map[string]bool{}
	for path := range source.files {
		expected[filepath.Clean(path)] = true
	}
	markerOwned := false
	markerItem := Item{
		ID: "skill-render:" + rendered.agent.Name + ":" + source.name + ":.regesto-owned", Kind: "skill-render",
		Owners: []string{rendered.agent.Name}, DeclaredTargets: []string{markerTarget}, CanonicalTarget: markerCanonical,
		IntendedState: "Regesto ownership marker for integration-specific generated skill", desired: marker, mode: 0o600,
	}
	current, readErr := os.ReadFile(markerCanonical)
	switch {
	case readErr == nil && bytes.Equal(current, marker):
		markerOwned = true
		markerItem.Action, markerItem.CurrentState = "current", "ownership marker current"
		markerItem.BackupAction, markerItem.DryRun = "none", "leave ownership marker unchanged"
	case readErr == nil:
		return fmt.Errorf("refuse rendered skill stage with mismatched ownership marker: %s", markerCanonical)
	case os.IsNotExist(readErr):
		adoptable, err := adoptableStage(dir, expected)
		if err != nil {
			return err
		}
		if !adoptable {
			return fmt.Errorf("refuse unowned rendered skill directory with unexpected files: %s", dir)
		}
		markerItem.Action, markerItem.CurrentState = "create", "ownership marker missing"
		markerItem.BackupAction, markerItem.DryRun = "none (Regesto-owned stage)", "create integration skill ownership marker"
	default:
		return readErr
	}
	p.Items = append(p.Items, markerItem)

	paths := make([]string, 0, len(source.files))
	for path := range source.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		body := source.files[rel]
		target := filepath.Join(dir, rel)
		if err := rejectSkillPayloadSymlinks(dir, rel); err != nil {
			return err
		}
		canonical, err := CanonicalTarget(target)
		if err != nil {
			return err
		}
		if !within(rendered.canonicalStage, canonical) {
			return fmt.Errorf("refuse rendered skill outside integration stage: %s", canonical)
		}
		item := Item{
			ID: "skill-render:" + rendered.agent.Name + ":" + source.name + ":" + filepath.ToSlash(rel), Kind: "skill-render",
			Owners: []string{rendered.agent.Name}, DeclaredTargets: []string{target}, CanonicalTarget: canonical,
			IntendedState: "portable skill rendered for integration " + rendered.agent.Name + " with variant " + rendered.variant,
			desired:       body, mode: 0o644, ownership: marker, ownershipTarget: markerCanonical,
		}
		current, err := os.ReadFile(canonical)
		switch {
		case err == nil && bytes.Equal(current, body):
			item.Action, item.CurrentState, item.BackupAction, item.DryRun = "current", "rendered file current", "none", "leave rendered skill file unchanged"
		case err == nil:
			item.Action, item.CurrentState, item.BackupAction, item.DryRun = "update", "rendered file stale", "none (Regesto-owned stage)", "refresh Regesto-owned rendered skill file"
			item.before = current
		case os.IsNotExist(err):
			item.Action, item.CurrentState, item.BackupAction, item.DryRun = "create", "rendered file missing", "none (target does not exist)", "create integration-specific rendered skill file"
		default:
			return err
		}
		p.Items = append(p.Items, item)
	}
	if markerOwned {
		return planStaleSkillFiles(p, rendered.agent.Name, source.name, dir, rendered.canonicalStage, expected, marker, []string{rendered.agent.Name})
	}
	return nil
}

func rejectSkillPayloadSymlinks(skillDir, rel string) error {
	current := skillDir
	for _, component := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse symlinked rendered skill payload path: %s", current)
		}
	}
	return nil
}

func planStaleSkillDirs(p *Plan, stage, canonicalStage string, sources []skillSource, owners []string, markerFor func(string) []byte) error {
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
		markerWant := markerFor(entry.Name())
		marker, markerErr := os.ReadFile(markerPath)
		if markerErr != nil || !bytes.Equal(marker, markerWant) {
			p.Items = append(p.Items, Item{
				ID: "skill-render-preserve:" + canonical, Kind: "skill-render", Action: "skip", Owners: []string{},
				DeclaredTargets: []string{target}, CanonicalTarget: canonical, CurrentState: "unshipped stage directory without a matching Regesto ownership marker",
				IntendedState: "foreign directory preserved", BackupAction: "none", DryRun: "leave unowned stage directory unchanged",
			})
			continue
		}
		digest, err := treeDigest(canonical)
		if err != nil {
			return err
		}
		p.Items = append(p.Items, Item{
			ID: "skill-render-prune:" + canonical, Kind: "skill-render-prune", Action: "remove", Owners: owners,
			DeclaredTargets: []string{target}, CanonicalTarget: canonical, CurrentState: "Regesto-owned rendered skill has no shipped source",
			IntendedState: "absent", BackupAction: "none (Regesto-owned generated stage)", DryRun: "remove stale Regesto-owned rendered skill directory",
			before: digest, ownership: markerWant, ownershipTarget: markerPath, renderRoot: canonicalStage,
		})
	}
	return nil
}

func planStaleSkillFiles(p *Plan, integration, skill, declaredDir, canonicalStage string, expected map[string]bool, marker []byte, owners []string) error {
	canonicalDir, err := CanonicalTarget(declaredDir)
	if err != nil {
		return err
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
		if !within(canonicalDir, canonical) || entry.Type()&os.ModeType != 0 {
			return fmt.Errorf("refuse to prune invalid generated skill entry %s", path)
		}
		body, err := os.ReadFile(canonical)
		if err != nil {
			return err
		}
		markerTarget := filepath.Join(canonicalDir, ".regesto-owned")
		p.Items = append(p.Items, Item{
			ID: "skill-render-file-prune:" + integration + ":" + skill + ":" + filepath.ToSlash(rel), Kind: "skill-render-file", Action: "remove",
			Owners: owners, DeclaredTargets: []string{filepath.Join(declaredDir, rel)}, CanonicalTarget: canonical,
			CurrentState: "generated skill file has no shipped source", IntendedState: "absent", BackupAction: "none (Regesto-owned generated stage)",
			DryRun: "remove stale generated skill file", before: body, ownership: marker, ownershipTarget: markerTarget, renderRoot: canonicalStage,
		})
		return nil
	})
}

func planSkillTargets(p *Plan, renders []integrationRender) error {
	type targetGroup struct {
		canonical string
		declared  []string
		renders   []integrationRender
	}
	groups := map[string]*targetGroup{}
	for _, rendered := range renders {
		for _, declared := range rendered.agent.SkillsDirs {
			canonical, err := CanonicalTarget(declared)
			if err != nil {
				return err
			}
			group := groups[canonical]
			if group == nil {
				group = &targetGroup{canonical: canonical}
				groups[canonical] = group
			}
			group.declared = append(group.declared, declared)
			group.renders = append(group.renders, rendered)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group := groups[key]
		sort.Slice(group.renders, func(i, j int) bool { return group.renders[i].agent.Name < group.renders[j].agent.Name })
		chosen := group.renders[0]
		owners := make([]string, 0, len(group.renders))
		for _, rendered := range group.renders {
			owners = append(owners, rendered.agent.Name)
			if rendered.variant != chosen.variant {
				return fmt.Errorf("skills target conflict at %s: integrations %q and %q render variants %q and %q; configure separate skills_dir targets", key, chosen.agent.Name, rendered.agent.Name, chosen.variant, rendered.variant)
			}
		}
		owners, group.declared = sortedUnique(owners), sortedUnique(group.declared)
		info, err := os.Stat(key)
		if err == nil && !info.IsDir() {
			return fmt.Errorf("skills target %s is not a directory", key)
		}
		if os.IsNotExist(err) {
			p.Items = append(p.Items, Item{
				ID: "skills-dir:" + key, Kind: "skills-directory", Action: "create", Owners: owners, DeclaredTargets: group.declared,
				CanonicalTarget: key, CurrentState: "missing", IntendedState: "skills directory present", BackupAction: "none (target does not exist)",
				DryRun: "create the shared skills directory", mode: 0o755,
			})
		} else if err != nil {
			return err
		}
		if err := planOwnedDanglingLinks(p, owners, group.declared, key, chosen.sources); err != nil {
			return err
		}
		for _, source := range chosen.sources {
			dest := filepath.Join(key, source.name)
			want := filepath.Join(chosen.stage, source.name)
			item, err := planSkillLink(owners, group.declared, dest, want, p.KBRoot)
			if err != nil {
				return err
			}
			p.Items = append(p.Items, item)
		}
	}
	return nil
}

func planOwnedDanglingLinks(p *Plan, owners, declaredDirs []string, canonicalDir string, sources []skillSource) error {
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
		if !regestoSkillLinkOwned(p.KBRoot, raw, entry.Name()) {
			continue
		}
		declared := make([]string, 0, len(declaredDirs))
		for _, dir := range declaredDirs {
			declared = append(declared, filepath.Join(dir, entry.Name()))
		}
		p.Items = append(p.Items, Item{
			ID: "skill-link-prune:" + dest, Kind: "skill-link", Action: "remove", Owners: owners,
			DeclaredTargets: sortedUnique(declared), CanonicalTarget: dest, CurrentState: "dangling Regesto-owned skill link to " + raw,
			IntendedState: "absent", BackupAction: "none (Regesto-owned symlink)", DryRun: "remove dangling Regesto-owned skill link", before: []byte(raw),
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
		ID: "skill-link:" + canonical, Kind: "skill-link", Owners: owners, DeclaredTargets: sortedUnique(declared), CanonicalTarget: canonical,
		IntendedState: "symbolic link to " + want, BackupAction: "none", linkTarget: want,
	}
	info, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		item.Action, item.CurrentState, item.DryRun = "create", "missing", "create integration-specific skill link"
		return item, nil
	}
	if err != nil {
		return Item{}, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		item.Action, item.CurrentState, item.DryRun = "skip", "foreign non-symlink entry preserved", "leave foreign skill entry unchanged"
		return item, nil
	}
	raw, err := os.Readlink(dest)
	if err != nil {
		return Item{}, err
	}
	resolved, resolveErr := filepath.EvalSymlinks(dest)
	wantResolved, wantErr := filepath.EvalSymlinks(want)
	if resolveErr == nil && wantErr == nil && resolved == wantResolved {
		item.Action, item.CurrentState, item.DryRun = "current", "owned skill link current", "leave skill link unchanged"
		return item, nil
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(filepath.Dir(dest), raw)
	}
	raw = filepath.Clean(raw)
	if regestoSkillLinkOwned(root, raw, filepath.Base(dest)) {
		item.Action, item.CurrentState, item.DryRun = "replace", "legacy or stale Regesto-owned skill link", "replace Regesto-owned skill link"
		item.before = []byte(raw)
		return item, nil
	}
	item.Action, item.CurrentState, item.DryRun = "skip", "foreign symlink to "+raw+" preserved", "leave foreign skill link unchanged"
	return item, nil
}

func regestoSkillLinkOwned(root, raw, skill string) bool {
	canonicalRoot, err := CanonicalTarget(root)
	if err != nil {
		return false
	}
	canonicalRaw, err := CanonicalTarget(raw)
	if err != nil {
		return false
	}
	legacySource := filepath.Join(canonicalRoot, "adapters", "skills", skill)
	if canonicalRaw == legacySource {
		return true
	}
	legacy := filepath.Join(canonicalRoot, ".state", "skills", skill)
	if canonicalRaw == legacy {
		marker, err := os.ReadFile(filepath.Join(canonicalRaw, ".regesto-owned"))
		return err == nil && bytes.Equal(marker, stageMarker(canonicalRoot, skill))
	}
	integrations := filepath.Join(canonicalRoot, ".state", "integrations")
	rel, err := filepath.Rel(integrations, canonicalRaw)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 3 || parts[1] != "skills" || parts[2] != skill {
		return false
	}
	marker, err := os.ReadFile(filepath.Join(canonicalRaw, ".regesto-owned"))
	return err == nil && bytes.Equal(marker, stageMarker(canonicalRoot, parts[0], skill))
}

func stageMarker(canonicalRoot string, parts ...string) []byte {
	if len(parts) == 1 {
		return []byte("regesto-generated-skill-stage-v1\nkb_root=" + canonicalRoot + "\nskill=" + parts[0] + "\n")
	}
	return []byte("regesto-generated-integration-skill-v1\nkb_root=" + canonicalRoot + "\nintegration=" + parts[0] + "\nskill=" + parts[1] + "\n")
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
	return hash.Sum(nil), err
}

func adoptableStage(dir string, expected map[string]bool) (bool, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil || !info.IsDir() {
		return false, err
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
				if candidate == rel || strings.HasPrefix(filepath.Clean(candidate), prefix) {
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

func canonicalLinkPath(path string) (string, error) {
	parent, err := CanonicalTarget(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}
