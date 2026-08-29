package adapters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/config"
)

const profileSchemaVersion = 1

var safeID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var safeCommand = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// Profile is the declarative, machine-independent description of an
// integration. Paths in profiles deliberately remain home-relative; resolved
// Agents contain the corresponding local paths.
type Profile struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	DisplayName   string         `json:"display_name"`
	Detect        Detection      `json:"detect"`
	Skills        Targets        `json:"skills"`
	Instructions  Instructions   `json:"instructions"`
	Hooks         []Hook         `json:"hooks"`
	Memory        []MemorySource `json:"memory"`
	DefaultTrust  string         `json:"default_trust"`
	ExcludeGlobs  []string       `json:"exclude_globs"`
}

type Detection struct {
	Paths    []string `json:"paths"`
	Commands []string `json:"commands"`
}

type Targets struct {
	Targets []string `json:"targets"`
	Variant string   `json:"variant"`
}

type Instructions struct {
	Targets []string `json:"targets"`
	Create  bool     `json:"create"`
}

type Hook struct {
	Protocol  string `json:"protocol"`
	Registrar string `json:"registrar"`
	Settings  string `json:"settings"`
}

type MemorySource struct {
	Kind     string `json:"kind"`
	Location string `json:"location"`
}

// Profiles loads the embedded profiles and then overlays instance-owned
// profiles. The latter makes a copied instance self-describing while embedded
// data keeps older instances (which have no profiles directory) working.
func Profiles(kbRoot string) (map[string]Profile, error) {
	profiles, err := loadProfileFS(regesto.Adapters, "adapters/profiles", "embedded adapters/profiles")
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(kbRoot, "adapters", "profiles")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return profiles, nil
		}
		return nil, fmt.Errorf("read instance profiles %s: %w", dir, err)
	}
	instanceIDs := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		p, err := parseProfile(body, path, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		if previous, exists := instanceIDs[p.ID]; exists {
			return nil, fmt.Errorf("instance profiles %s and %s: duplicate profile id %q", previous, entry.Name(), p.ID)
		}
		instanceIDs[p.ID] = entry.Name()
		profiles[p.ID] = p
	}
	return profiles, nil
}

func loadProfileFS(fsys fs.FS, dir, label string) (map[string]Profile, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	out := make(map[string]Profile, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		path := filepath.ToSlash(filepath.Join(dir, entry.Name()))
		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, err
		}
		p, err := parseProfile(body, label+"/"+entry.Name(), name)
		if err != nil {
			return nil, err
		}
		if _, exists := out[p.ID]; exists {
			return nil, fmt.Errorf("%s: duplicate profile id %q", label, p.ID)
		}
		out[p.ID] = p
	}
	return out, nil
}

func embeddedProfiles() (map[string]Profile, error) {
	return loadProfileFS(regesto.Adapters, "adapters/profiles", "embedded adapters/profiles")
}

func parseProfile(body []byte, source, fileID string) (Profile, error) {
	var p Profile
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return p, fmt.Errorf("%s: profile must be exactly one JSON object", source)
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return p, fmt.Errorf("%s: invalid JSON: %w", source, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return p, fmt.Errorf("%s: profile must contain exactly one JSON object", source)
		}
		return p, fmt.Errorf("%s: invalid trailing JSON: %w", source, err)
	}
	if err := validateProfile(p, source, fileID); err != nil {
		return p, err
	}
	return p, nil
}

func validateProfile(p Profile, source, fileID string) error {
	if p.SchemaVersion != profileSchemaVersion {
		return fmt.Errorf("%s: schema_version must be %d, got %d", source, profileSchemaVersion, p.SchemaVersion)
	}
	if !safeID.MatchString(p.ID) {
		return fmt.Errorf("%s: profile id %q must match %s", source, p.ID, safeID.String())
	}
	if p.ID != fileID {
		return fmt.Errorf("%s: file id %q does not match profile id %q", source, fileID, p.ID)
	}
	if strings.TrimSpace(p.DisplayName) == "" {
		return fmt.Errorf("%s: display_name must not be empty", source)
	}
	if p.Skills.Variant != "" && !safeID.MatchString(p.Skills.Variant) {
		return fmt.Errorf("%s: skills.variant %q must match %s", source, p.Skills.Variant, safeID.String())
	}
	if len(p.Skills.Targets) > 0 && p.Skills.Variant == "" {
		return fmt.Errorf("%s: skills.variant must be nonempty when skills.targets are declared", source)
	}
	for _, path := range append(append(append([]string{}, p.Detect.Paths...), p.Skills.Targets...), p.Instructions.Targets...) {
		if err := validateProfilePath(source, path); err != nil {
			return err
		}
	}
	for _, command := range p.Detect.Commands {
		if strings.TrimSpace(command) == "" || command != strings.TrimSpace(command) || !safeCommand.MatchString(command) {
			return fmt.Errorf("%s: detect command %q must be a nonempty bare executable name", source, command)
		}
	}
	noneHooks := 0
	for _, h := range p.Hooks {
		if !oneOf(h.Protocol, "claude-session-start-v1", "hermes-pre-llm-v1", "none") {
			return fmt.Errorf("%s: unknown hook protocol %q", source, h.Protocol)
		}
		if !oneOf(h.Registrar, "claude-settings-json-v1", "hermes-config-yaml-v1", "manual", "none") {
			return fmt.Errorf("%s: unknown hook registrar %q", source, h.Registrar)
		}
		if h.Settings != "" {
			if err := validateProfilePath(source, h.Settings); err != nil {
				return err
			}
		}
		if h.Protocol == "none" {
			noneHooks++
			if h.Registrar != "none" || h.Settings != "" {
				return fmt.Errorf("%s: protocol none requires registrar none and no settings", source)
			}
		}
		switch h.Registrar {
		case "claude-settings-json-v1":
			if h.Protocol != "claude-session-start-v1" || h.Settings == "" {
				return fmt.Errorf("%s: claude-settings-json-v1 requires claude-session-start-v1 and settings", source)
			}
		case "hermes-config-yaml-v1":
			if h.Protocol != "hermes-pre-llm-v1" || h.Settings == "" {
				return fmt.Errorf("%s: hermes-config-yaml-v1 requires hermes-pre-llm-v1 and settings", source)
			}
		case "none":
			if h.Protocol != "none" || h.Settings != "" {
				return fmt.Errorf("%s: registrar none requires protocol none and no settings", source)
			}
		}
	}
	if noneHooks > 0 && len(p.Hooks) != 1 {
		return fmt.Errorf("%s: protocol none hook cannot be combined with other hooks", source)
	}
	noneMemory := 0
	for _, m := range p.Memory {
		if !safeID.MatchString(m.Kind) {
			return fmt.Errorf("%s: memory kind %q must match %s", source, m.Kind, safeID.String())
		}
		if m.Kind == "markdown-glob-v1" && m.Location == "" {
			return fmt.Errorf("%s: markdown-glob-v1 memory needs a location", source)
		}
		if m.Kind == "none" && m.Location != "" {
			return fmt.Errorf("%s: none memory must not declare a location", source)
		}
		if m.Kind == "none" {
			noneMemory++
		}
		if m.Location != "" {
			if err := validateProfilePath(source, m.Location); err != nil {
				return err
			}
		}
	}
	if noneMemory > 0 && len(p.Memory) != 1 {
		return fmt.Errorf("%s: memory kind none cannot be combined with other memory sources", source)
	}
	if !oneOf(p.DefaultTrust, "supervised", "quarantine") {
		return fmt.Errorf("%s: unknown default_trust %q", source, p.DefaultTrust)
	}
	return nil
}

func validateProfilePath(source, p string) error {
	if filepath.IsAbs(p) || !strings.HasPrefix(p, "~/") {
		return fmt.Errorf("%s: profile path %q must be home-relative (~/...) and not a personal absolute path", source, p)
	}
	suffix := filepath.Clean(strings.TrimPrefix(p, "~/"))
	if filepath.IsAbs(suffix) || suffix == "." || suffix == ".." || strings.HasPrefix(suffix, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s: profile path %q must not escape the home directory", source, p)
	}
	return nil
}

func oneOf(v string, choices ...string) bool {
	for _, choice := range choices {
		if v == choice {
			return true
		}
	}
	return false
}

// Resolve converts the configured integration list into concrete targets.
// Legacy unknown agents deliberately remain empty, preserving the old warning
// path; only new-vocabulary unknown integrations inherit the generic template.
func Resolve(cfg *config.Config) ([]Agent, error) {
	return ResolveFrom(cfg, cfg.KBRoot)
}

// ResolveFrom resolves config targets while loading declarative profiles from
// profileRoot. Normal installs use the instance itself. Upgrade dry-runs use a
// temporary post-upgrade source view so their adapter plan matches the files a
// real upgrade would retain, refresh, or remove without mutating the instance.
func ResolveFrom(cfg *config.Config, profileRoot string) ([]Agent, error) {
	profiles, err := Profiles(profileRoot)
	if err != nil {
		return nil, err
	}
	ids := cfg.IntegrationIDs()
	if cfg.UsesLegacyAgents() {
		for section := range cfg.Sections {
			if strings.HasPrefix(section, "integrations.") {
				return nil, fmt.Errorf("config uses legacy agents; use either agents or integrations, not [%s]", section)
			}
		}
	}
	seen := map[string]bool{}
	out := make([]Agent, 0, len(ids))
	for _, id := range ids {
		// `human` is the protocol's authoritative principal, not an agent
		// integration. Allowing a configured/harvested integration to claim this
		// ID would let its captures be stamped as human assertions.
		if id == "human" {
			return nil, fmt.Errorf("config integration id %q is reserved for human authority", id)
		}
		if seen[id] && !cfg.UsesLegacyAgents() {
			return nil, fmt.Errorf("config lists integration %q more than once", id)
		}
		seen[id] = true
		p, found := profiles[id]
		if !found && cfg.UsesLegacyAgents() {
			a := legacyUnknown(cfg, id)
			applyLegacyOverrides(cfg, &a)
			out = append(out, a)
			continue
		}
		if !safeID.MatchString(id) {
			return nil, fmt.Errorf("config integration id %q must match %s", id, safeID.String())
		}
		values := cfg.Section("integrations." + id)
		profileID := values["profile"]
		if profileID == "" {
			profileID = id
			if !found {
				profileID = "generic"
			}
		}
		if !found || profileID != id {
			var ok bool
			p, ok = profiles[profileID]
			if !ok {
				return nil, fmt.Errorf("integration %q references unknown profile %q", id, profileID)
			}
		}
		a, err := resolveProfile(cfg, id, p, values)
		if err != nil {
			return nil, err
		}
		if cfg.UsesLegacyAgents() {
			applyLegacyOverrides(cfg, &a)
		}
		out = append(out, a)
	}
	if err := validateIntegrationSections(cfg, seen); err != nil {
		return nil, err
	}
	return out, nil
}

func resolveProfile(cfg *config.Config, id string, p Profile, values map[string]string) (Agent, error) {
	a := Agent{
		Name:               id,
		ProfileID:          p.ID,
		DisplayName:        p.DisplayName,
		Detect:             expandDetection(p.Detect),
		Hooks:              expandHooks(p.Hooks),
		MemorySources:      expandMemory(p.Memory),
		DefaultTrust:       p.DefaultTrust,
		MaxCaptureBytes:    maxCaptureBytes(cfg),
		ExcludeGlobs:       excludes(cfg, id, p.ExcludeGlobs),
		SkillsDirs:         expandPaths(p.Skills.Targets),
		InstructionsFiles:  expandPaths(p.Instructions.Targets),
		SkillsVariant:      p.Skills.Variant,
		InstructionsCreate: p.Instructions.Create,
	}
	if err := validateIntegrationOverrideKeys(id, values); err != nil {
		return a, err
	}
	if v := values["skills_dir"]; v != "" {
		a.SkillsDirs = []string{expandHome(v)}
	}
	if v := values["instructions_file"]; v != "" {
		a.InstructionsFiles = []string{expandHome(v)}
	}
	if v := values["settings_file"]; v != "" {
		if err := setNewHookSettings(&a, expandHome(v)); err != nil {
			return a, fmt.Errorf("integration %q: %w", id, err)
		}
	}
	if v := values["memory_kind"]; v != "" {
		if !safeID.MatchString(v) {
			return a, fmt.Errorf("integration %q: memory_kind %q must match %s", id, v, safeID.String())
		}
		location, err := expandIntegrationMemoryLocation(id, values["memory_location"])
		if err != nil {
			return a, err
		}
		a.MemorySources = []MemorySource{{Kind: v, Location: location}}
	}
	if v := values["memory_location"]; v != "" && values["memory_kind"] == "" {
		location, err := expandIntegrationMemoryLocation(id, v)
		if err != nil {
			return a, err
		}
		var matches []int
		for i, source := range a.MemorySources {
			if source.Kind == "markdown-glob-v1" {
				matches = append(matches, i)
			}
		}
		if len(matches) == 0 {
			return a, fmt.Errorf("integration %q: memory_location requires memory_kind markdown-glob-v1", id)
		}
		if len(matches) != 1 {
			return a, fmt.Errorf("integration %q: memory_location is ambiguous across %d markdown sources; set memory_kind", id, len(matches))
		}
		a.MemorySources[matches[0]].Location = location
	}
	if v := values["trust"]; v != "" {
		if !oneOf(v, "supervised", "quarantine") {
			return a, fmt.Errorf("integration %q: unknown trust %q", id, v)
		}
		a.DefaultTrust = v
	}
	if len(a.SkillsDirs) > 0 {
		a.SkillsDir = a.SkillsDirs[0]
	}
	if len(a.InstructionsFiles) > 0 {
		a.InstructionsFile = a.InstructionsFiles[0]
	}
	for _, m := range a.MemorySources {
		if m.Kind == "markdown-glob-v1" {
			a.MemoryGlob = m.Location
			break
		}
	}
	if err := validateResolvedMemory(id, a.MemorySources); err != nil {
		return a, err
	}
	// SettingsFile is the legacy flat Claude-settings compatibility field. New
	// registrars expose their targets through Hooks so adding one does not change
	// established `regesto config` text output for existing integrations.
	if len(a.Hooks) > 0 && a.Hooks[0].Registrar == "claude-settings-json-v1" {
		a.SettingsFile = a.Hooks[0].Settings
	}
	return a, nil
}

func legacyUnknown(cfg *config.Config, id string) Agent {
	return Agent{Name: id, ProfileID: "", DisplayName: id, MaxCaptureBytes: maxCaptureBytes(cfg), ExcludeGlobs: excludes(cfg, id, nil)}
}

func applyLegacyOverrides(cfg *config.Config, a *Agent) {
	if v := cfg.Section("skills_dirs")[a.Name]; v != "" {
		a.SkillsDir, a.SkillsDirs = expandHome(v), []string{expandHome(v)}
	}
	if v := cfg.Section("instructions")[a.Name]; v != "" {
		a.InstructionsFile, a.InstructionsFiles = expandHome(v), []string{expandHome(v)}
	}
	if v := cfg.Section("settings_files")[a.Name]; v != "" {
		setHookSettings(a, expandHome(v))
	}
	if v := cfg.Section("memory_dirs")[a.Name]; v != "" {
		a.MemoryGlob = expandHome(v)
		if !filepath.IsAbs(a.MemoryGlob) {
			a.MemoryGlob = filepath.Join(cfg.KBRoot, a.MemoryGlob)
		}
		a.MemorySources = []MemorySource{{Kind: "markdown-glob-v1", Location: a.MemoryGlob}}
	}
}

func setHookSettings(a *Agent, settings string) {
	a.SettingsFile = settings
	if len(a.Hooks) > 0 {
		a.Hooks[0].Settings = settings
	}
}

func setNewHookSettings(a *Agent, settings string) error {
	var matches []int
	for i, hook := range a.Hooks {
		if hook.Protocol != "none" && hook.Registrar != "none" {
			matches = append(matches, i)
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("settings_file requires one compatible non-none hook; profile %q has none", a.ProfileID)
	}
	if len(matches) != 1 {
		return fmt.Errorf("settings_file is ambiguous: profile %q has %d compatible hooks", a.ProfileID, len(matches))
	}
	a.Hooks[matches[0]].Settings = settings
	a.SettingsFile = settings
	return nil
}

func validateResolvedMemory(id string, memory []MemorySource) error {
	for _, source := range memory {
		if !safeID.MatchString(source.Kind) {
			return fmt.Errorf("integration %q: memory kind %q must match %s", id, source.Kind, safeID.String())
		}
		switch source.Kind {
		case "markdown-glob-v1":
			if source.Location == "" {
				return fmt.Errorf("integration %q: markdown-glob-v1 memory requires memory_location", id)
			}
		case "none":
			if source.Location != "" {
				return fmt.Errorf("integration %q: none memory forbids memory_location", id)
			}
		}
	}
	return nil
}

func expandIntegrationMemoryLocation(id, location string) (string, error) {
	if location == "" {
		return "", nil
	}
	expanded := expandHome(location)
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("integration %q: memory_location %q must be absolute or home-relative (~/...)", id, location)
	}
	return expanded, nil
}

func validateIntegrationOverrideKeys(id string, values map[string]string) error {
	for key := range values {
		if !oneOf(key, "profile", "skills_dir", "instructions_file", "settings_file", "memory_kind", "memory_location", "trust") {
			return fmt.Errorf("integration %q: unknown override key %q", id, key)
		}
	}
	return nil
}

func validateIntegrationSections(cfg *config.Config, listed map[string]bool) error {
	for section := range cfg.Sections {
		if !strings.HasPrefix(section, "integrations.") {
			continue
		}
		id := strings.TrimPrefix(section, "integrations.")
		if !listed[id] {
			return fmt.Errorf("integration override section [%s] names an integration not listed in integrations", section)
		}
	}
	return nil
}

func expandPaths(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = expandHome(p)
	}
	return out
}

func expandDetection(d Detection) Detection {
	d.Paths = expandPaths(d.Paths)
	d.Commands = append([]string(nil), d.Commands...)
	return d
}

func expandHooks(hooks []Hook) []Hook {
	out := append([]Hook(nil), hooks...)
	for i := range out {
		out[i].Settings = expandHome(out[i].Settings)
	}
	return out
}

func expandMemory(memory []MemorySource) []MemorySource {
	out := append([]MemorySource(nil), memory...)
	for i := range out {
		out[i].Location = expandHome(out[i].Location)
	}
	return out
}

// ProfileIDs is useful to deterministic machine-facing output and tests.
func ProfileIDs(kbRoot string) ([]string, error) {
	profiles, err := Profiles(kbRoot)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// DetectProfiles reports profiles detected from their declarative path or
// command signals. Unlike Detect, it includes instance profile overrides and
// treats commands as an explicit signal; legacy init intentionally remains
// path-only through Detect.
func DetectProfiles(kbRoot string) ([]string, error) {
	profiles, err := Profiles(kbRoot)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var detected []string
	for _, id := range ids {
		p := profiles[id]
		found := false
		for _, path := range p.Detect.Paths {
			if _, err := os.Stat(expandHome(path)); err == nil {
				found = true
				break
			}
		}
		if !found {
			for _, command := range p.Detect.Commands {
				if _, err := exec.LookPath(command); err == nil {
					found = true
					break
				}
			}
		}
		if found {
			detected = append(detected, id)
		}
	}
	return detected, nil
}
