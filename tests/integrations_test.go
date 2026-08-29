package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/normalize"
)

func configFixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "tests", "fixtures", "config", name)
}

func loadConfigFixture(t *testing.T, name string) *config.Config {
	t.Helper()
	cfg, err := config.Load(configFixture(t, name))
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return cfg
}

func TestLegacyBuiltInIntegrationDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadConfigFixture(t, "legacy-defaults.toml")
	got := adapters.For(cfg)
	if len(got) != 3 {
		t.Fatalf("got %d adapters, want 3", len(got))
	}
	want := map[string]adapters.Agent{
		"claude": {Name: "claude", SkillsDir: filepath.Join(home, ".claude/skills"), InstructionsFile: filepath.Join(home, ".claude/CLAUDE.md"), SettingsFile: filepath.Join(home, ".claude/settings.json"), MemoryGlob: filepath.Join(home, ".claude/projects/*/memory"), MaxCaptureBytes: 10 * 1024 * 1024},
		"codex":  {Name: "codex", SkillsDir: filepath.Join(home, ".codex/skills"), InstructionsFile: filepath.Join(home, ".codex/AGENTS.md"), MemoryGlob: filepath.Join(home, ".codex/memories"), MaxCaptureBytes: 10 * 1024 * 1024, ExcludeGlobs: []string{"raw_memories.md"}},
		"hermes": {Name: "hermes", SkillsDir: filepath.Join(home, ".hermes/skills"), InstructionsFile: filepath.Join(home, ".hermes/SOUL.md"), MemoryGlob: filepath.Join(home, ".hermes/memories"), MaxCaptureBytes: 10 * 1024 * 1024},
	}
	for _, gotAgent := range got {
		wantAgent, ok := want[gotAgent.Name]
		if !ok {
			t.Errorf("unexpected adapter %q", gotAgent.Name)
			continue
		}
		if gotAgent.Name != wantAgent.Name || gotAgent.SkillsDir != wantAgent.SkillsDir || gotAgent.InstructionsFile != wantAgent.InstructionsFile || gotAgent.SettingsFile != wantAgent.SettingsFile || gotAgent.MemoryGlob != wantAgent.MemoryGlob || gotAgent.MaxCaptureBytes != wantAgent.MaxCaptureBytes || strings.Join(gotAgent.ExcludeGlobs, "\x00") != strings.Join(wantAgent.ExcludeGlobs, "\x00") {
			t.Errorf("%s resolved as %+v, want %+v", gotAgent.Name, gotAgent, wantAgent)
		}
	}
}

func TestAdapterLegacyOverrideTablesWin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadConfigFixture(t, "legacy-overrides.toml")
	byName := map[string]adapters.Agent{}
	for _, a := range adapters.For(cfg) {
		byName[a.Name] = a
	}
	if got := byName["claude"].SkillsDir; got != filepath.Join(home, ".legacy/claude-skills") {
		t.Errorf("legacy skills override = %q", got)
	}
	if got := byName["codex"].InstructionsFile; got != filepath.Join(home, ".legacy/AGENTS.md") {
		t.Errorf("legacy instructions override = %q", got)
	}
	if got := byName["claude"].SettingsFile; got != filepath.Join(home, ".legacy/claude-settings.json") {
		t.Errorf("legacy settings override = %q", got)
	}
	if got := byName["claude"].Hooks[0].Settings; got != byName["claude"].SettingsFile {
		t.Errorf("legacy settings override disagrees with hook metadata: hook=%q flat=%q", got, byName["claude"].SettingsFile)
	}
	if got := byName["hermes"].MemoryGlob; got != filepath.Join(home, ".legacy/hermes-memory") {
		t.Errorf("legacy memory override = %q", got)
	}
	if got := byName["codex"].SkillsDir; got != filepath.Join(home, ".codex/skills") {
		t.Errorf("unoverridden default changed = %q", got)
	}
}

func TestAdapterLegacyDetectionIdentifiers(t *testing.T) {
	wantAgents := []string{"claude", "codex", "hermes"}
	if got := adapters.KnownAgents(); strings.Join(got, ",") != strings.Join(wantAgents, ",") {
		t.Fatalf("known agents = %v, want exactly %v", got, wantAgents)
	}
	for _, tc := range []struct {
		name, home string
		dirs, want []string
	}{
		{name: "none", home: t.TempDir()},
		{name: "one", home: t.TempDir(), dirs: []string{".claude"}, want: []string{"claude"}},
		{name: "all", home: t.TempDir(), dirs: []string{".claude", ".codex", ".hermes"}, want: wantAgents},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			for _, dir := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(tc.home, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			got := adapters.Detect()
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("detected %v, want %v", got, tc.want)
			}
		})
	}
	// A second HOME must not retain detections from the first one.
	second := t.TempDir()
	t.Setenv("HOME", second)
	if err := os.MkdirAll(filepath.Join(second, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := adapters.Detect()
	sort.Strings(got)
	if strings.Join(got, ",") != "codex" {
		t.Errorf("second HOME detected %v, want [codex]", got)
	}
}

func TestIntegrationLegacyConfigTextOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	runConfig := func(fixture string) string {
		t.Helper()
		cmd := exec.Command("go", "run", "./cmd/regesto", "--config", configFixture(t, fixture), "config")
		cmd.Dir = repoRoot(t)
		cmd.Env = append(os.Environ(), "HOME="+home, "REGESTO_MACHINE=fixture-box")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("config command: %v\nstderr: %s", err, stderr.String())
		}
		return stdout.String()
	}
	want := strings.Join([]string{
		"kb_root=" + filepath.Join(repoRoot(t), "tests/fixtures/config"),
		"machine=fixture-box",
		"machine_source=env:REGESTO_MACHINE",
		"agents=claude codex hermes",
		"agent.claude.skills_dir=" + filepath.Join(home, ".claude/skills"),
		"agent.claude.instructions=" + filepath.Join(home, ".claude/CLAUDE.md"),
		"agent.claude.settings=" + filepath.Join(home, ".claude/settings.json"),
		"agent.codex.skills_dir=" + filepath.Join(home, ".codex/skills"),
		"agent.codex.instructions=" + filepath.Join(home, ".codex/AGENTS.md"),
		"agent.codex.settings=",
		"agent.hermes.skills_dir=" + filepath.Join(home, ".hermes/skills"),
		"agent.hermes.instructions=" + filepath.Join(home, ".hermes/SOUL.md"),
		"agent.hermes.settings=",
	}, "\n") + "\n"
	if got := runConfig("legacy-defaults.toml"); got != want {
		t.Errorf("config output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// Legacy override tables must be reflected in the same stable text format,
	// while values without overrides retain their built-in defaults.
	wantOverrides := strings.Join([]string{
		"kb_root=" + filepath.Join(repoRoot(t), "tests/fixtures/config"),
		"machine=fixture-box",
		"machine_source=env:REGESTO_MACHINE",
		"agents=claude codex hermes",
		"agent.claude.skills_dir=" + filepath.Join(home, ".legacy/claude-skills"),
		"agent.claude.instructions=" + filepath.Join(home, ".claude/CLAUDE.md"),
		"agent.claude.settings=" + filepath.Join(home, ".legacy/claude-settings.json"),
		"agent.codex.skills_dir=" + filepath.Join(home, ".codex/skills"),
		"agent.codex.instructions=" + filepath.Join(home, ".legacy/AGENTS.md"),
		"agent.codex.settings=",
		"agent.hermes.skills_dir=" + filepath.Join(home, ".hermes/skills"),
		"agent.hermes.instructions=" + filepath.Join(home, ".hermes/SOUL.md"),
		"agent.hermes.settings=",
	}, "\n") + "\n"
	if got := runConfig("legacy-overrides.toml"); got != wantOverrides {
		t.Errorf("override config output mismatch:\ngot:\n%s\nwant:\n%s", got, wantOverrides)
	}
}

func TestLegacyTrustBehavior(t *testing.T) {
	for _, tc := range []struct {
		agent, source string
		quarantined   bool
	}{
		{agent: "claude", source: "claude@fixture-box"},
		{agent: "codex", source: "codex@fixture-box"},
		{agent: "hermes", source: "hermes@fixture-box", quarantined: true},
	} {
		c := normalize.Capture{Agent: tc.agent, Source: tc.source}
		if got := c.Quarantined(map[string]bool{}); got != tc.quarantined {
			t.Errorf("%s quarantine = %v, want %v", tc.agent, got, tc.quarantined)
		}
		if tc.agent == "hermes" && c.Quarantined(map[string]bool{tc.source: true}) {
			t.Error("explicitly trusted Hermes source remained quarantined")
		}
	}
}

func TestIntegrationGenericProfileTargetContract(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadConfigFixture(t, "generic-profile.toml")
	got, err := adapters.Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved %d integrations, want 1", len(got))
	}
	a := got[0]
	if a.Name != "synthetic" || a.ProfileID != "generic" || a.DefaultTrust != "quarantine" {
		t.Errorf("generic integration = %+v", a)
	}
	if want := filepath.Join(home, ".synthetic", "skills"); a.SkillsDir != want {
		t.Errorf("skills = %q, want %q", a.SkillsDir, want)
	}
	if want := filepath.Join(home, ".synthetic", "memory"); a.MemoryGlob != want {
		t.Errorf("memory = %q, want %q", a.MemoryGlob, want)
	}
}

func TestIntegrationUnknownIDDefaultsToGenericProfile(t *testing.T) {
	cfg := writeConfig(t, "integrations = [\"arbitrary.agent\"]\n")
	got, err := adapters.Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "arbitrary.agent" || got[0].ProfileID != "generic" || got[0].DefaultTrust != "quarantine" {
		t.Errorf("unknown integration did not use generic profile: %+v", got)
	}
}

func TestConfigRejectsAgentsAndIntegrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("agents = [\"claude\"]\nintegrations = [\"codex\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil || !strings.Contains(err.Error(), "either agents or integrations") {
		t.Fatalf("error = %v, want actionable both-key rejection", err)
	}
}

func TestLegacyAgentsRejectIntegrationOverrideSections(t *testing.T) {
	cfg := writeConfig(t, "agents = [\"claude\"]\n[integrations.claude]\nskills_dir = \"/tmp/nope\"\n")
	if _, err := adapters.Resolve(cfg); err == nil || !strings.Contains(err.Error(), "use either agents or integrations") {
		t.Fatalf("error = %v, want vocabulary rejection", err)
	}
}

func TestConfigWithoutListKeepsLegacyTextOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("machine = \"fixture-box\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./cmd/regesto", "--config", path, "config")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "REGESTO_MACHINE=fixture-box")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("config: %v\\nstderr: %s", err, stderr.String())
	}
	want := "kb_root=" + root + "\nmachine=fixture-box\nmachine_source=env:REGESTO_MACHINE\nagents=\n"
	if got := stdout.String(); got != want {
		t.Errorf("config output = %q, want %q", got, want)
	}
}

func TestExplicitEmptyIntegrationsUsesNewVocabulary(t *testing.T) {
	cfg := writeConfig(t, "integrations = []\n")
	if cfg.UsesLegacyAgents() {
		t.Fatal("explicit integrations=[] must not fall back to legacy vocabulary")
	}
	if got := cfg.IntegrationIDs(); len(got) != 0 {
		t.Errorf("integration ids = %v, want empty", got)
	}
	programmatic := &config.Config{Integrations: []string{"synthetic"}}
	if programmatic.UsesLegacyAgents() || strings.Join(programmatic.IntegrationIDs(), ",") != "synthetic" {
		t.Errorf("programmatic integrations chose wrong vocabulary: %+v", programmatic)
	}
}

func TestIntegrationProfileValidationErrorsAreActionable(t *testing.T) {
	root := t.TempDir()
	writeProfile := func(t *testing.T, name, body string) *config.Config {
		t.Helper()
		dir := filepath.Join(root, "adapters", "profiles")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, entry := range []string{"bad.json", "wrong.json", "path.json", "typo.json", "traversal.json", "double-slash.json"} {
			_ = os.Remove(filepath.Join(dir, entry))
		}
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return writeConfig(t, "kb_root = \""+root+"\"\nintegrations = [\"synthetic\"]\n[integrations.synthetic]\nprofile = \"generic\"\n")
	}
	for _, tc := range []struct{ name, file, body, want string }{
		{"kind", "bad", `{"schema_version":1,"id":"bad","display_name":"Bad","skills":{"targets":[]},"instructions":{"targets":[]},"memory":[{"kind":"sqlite"}],"default_trust":"quarantine"}`, "unknown memory kind"},
		{"path", "path", `{"schema_version":1,"id":"path","display_name":"Path","skills":{"targets":["/Users/someone/skills"],"variant":"portable"},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "home-relative"},
		{"id", "wrong", `{"schema_version":1,"id":"different","display_name":"Wrong","skills":{"targets":[]},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "does not match"},
		{"typo", "typo", `{"schema_version":1,"id":"typo","display_name":"Typo","skills":{"targets":[]},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine","display_nam":"misspelled"}`, "unknown field"},
		{"traversal", "traversal", `{"schema_version":1,"id":"traversal","display_name":"Traversal","skills":{"targets":["~/../outside"],"variant":"portable"},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "must not escape"},
		{"hook", "bad", `{"schema_version":1,"id":"bad","display_name":"Bad","skills":{"targets":[]},"instructions":{"targets":[]},"hooks":[{"protocol":"claude-session-start-v1","registrar":"none"}],"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "registrar none requires"},
		{"command", "bad", `{"schema_version":1,"id":"bad","display_name":"Bad","detect":{"commands":["../not-a-command"]},"skills":{"targets":[]},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "bare executable name"},
		{"command-leading-dash", "bad", `{"schema_version":1,"id":"bad","display_name":"Bad","detect":{"commands":["-command"]},"skills":{"targets":[]},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "bare executable name"},
		{"command-leading-dot", "bad", `{"schema_version":1,"id":"bad","display_name":"Bad","detect":{"commands":[".command"]},"skills":{"targets":[]},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "bare executable name"},
		{"command-leading-plus", "bad", `{"schema_version":1,"id":"bad","display_name":"Bad","detect":{"commands":["+command"]},"skills":{"targets":[]},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "bare executable name"},
		{"none-hook", "bad", `{"schema_version":1,"id":"bad","display_name":"Bad","skills":{"targets":[]},"instructions":{"targets":[]},"hooks":[{"protocol":"none","registrar":"none"},{"protocol":"claude-session-start-v1","registrar":"manual"}],"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "cannot be combined"},
		{"none-memory", "bad", `{"schema_version":1,"id":"bad","display_name":"Bad","skills":{"targets":[]},"instructions":{"targets":[]},"memory":[{"kind":"none"},{"kind":"markdown-glob-v1","location":"~/.bad/memory"}],"default_trust":"quarantine"}`, "cannot be combined"},
		{"double-slash", "double-slash", `{"schema_version":1,"id":"double-slash","display_name":"Double","skills":{"targets":["~//outside"],"variant":"portable"},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "must not escape"},
		{"inactive-variant", "bad", `{"schema_version":1,"id":"bad","display_name":"Bad","skills":{"targets":[],"variant":"../host"},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"}`, "skills.variant"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := writeProfile(t, tc.file, tc.body)
			if _, err := adapters.Resolve(cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestIntegrationProfileParserRejectsNonObjectAndTrailingJSON(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "adapters", "profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, "kb_root = \""+root+"\"\nintegrations = [\"synthetic\"]\n[integrations.synthetic]\nprofile = \"generic\"\n")
	for _, body := range []string{"", "[]", `{"schema_version":1,"id":"parser","display_name":"Parser","skills":{"targets":[]},"instructions":{"targets":[]},"memory":[{"kind":"none"}],"default_trust":"quarantine"} {}`} {
		if err := os.WriteFile(filepath.Join(dir, "parser.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := adapters.Resolve(cfg); err == nil {
			t.Errorf("parser accepted %q", body)
		}
	}
}

func TestIntegrationSettingsFileRequiresOneCompatibleHook(t *testing.T) {
	hookless := writeConfig(t, "integrations = [\"generic\"]\n[integrations.generic]\nsettings_file = \"/tmp/settings\"\n")
	if _, err := adapters.Resolve(hookless); err == nil || !strings.Contains(err.Error(), "requires one compatible") {
		t.Fatalf("hookless settings_file error = %v", err)
	}

	claude := writeConfig(t, "integrations = [\"claude\"]\n[integrations.claude]\nsettings_file = \"/tmp/settings\"\n")
	resolved, err := adapters.Resolve(claude)
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0].SettingsFile != "/tmp/settings" || resolved[0].Hooks[0].Settings != "/tmp/settings" {
		t.Errorf("settings override did not update the sole hook: %+v", resolved[0])
	}
}

func TestIntegrationInstanceProfileOverridesEmbeddedProfile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "adapters", "profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version":1,"id":"claude","display_name":"Local Claude","detect":{"paths":["~/.local-claude"]},"skills":{"targets":["~/.local-claude/skills"],"variant":"portable"},"instructions":{"targets":["~/.local-claude/INSTRUCTIONS.md"]},"hooks":[{"protocol":"none","registrar":"none"}],"memory":[{"kind":"none"}],"default_trust":"quarantine"}`
	if err := os.WriteFile(filepath.Join(dir, "claude.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, "kb_root = \""+root+"\"\nintegrations = [\"claude\"]\n")
	got, err := adapters.Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].DisplayName != "Local Claude" || got[0].DefaultTrust != "quarantine" || !strings.Contains(got[0].SkillsDir, ".local-claude") || got[0].Hooks[0].Protocol != "none" {
		t.Errorf("instance profile did not override: %+v", got[0])
	}
}

func TestIntegrationResolvedBuiltInsHaveMetadataWithoutPersonalPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := writeConfig(t, "integrations = [\"claude\", \"codex\", \"hermes\"]\n")
	got, err := adapters.Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got {
		if a.ProfileID == "" || a.DisplayName == "" || a.DefaultTrust == "" {
			t.Errorf("missing metadata: %+v", a)
		}
		for _, p := range append(append([]string{}, a.SkillsDirs...), a.InstructionsFiles...) {
			if !strings.HasPrefix(p, home) || strings.Contains(p, "Workspace") {
				t.Errorf("resolved path %q is not portable", p)
			}
		}
	}
}

func TestNewConfigOverridesProfileDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := writeConfig(t, "integrations = [\"codex\"]\n[integrations.codex]\nskills_dir = \"/tmp/custom-skills\"\ninstructions_file = \"~/.custom/AGENTS.md\"\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \"/tmp/memory/*\"\ntrust = \"quarantine\"\n")
	got, err := adapters.Resolve(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := got[0]
	if a.SkillsDir != "/tmp/custom-skills" || a.MemoryGlob != "/tmp/memory/*" || a.DefaultTrust != "quarantine" || !strings.Contains(a.InstructionsFile, ".custom") {
		t.Errorf("overrides not applied: %+v", a)
	}
}

func TestIntegrationOverrideValidation(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"unknown-key", "integrations = [\"codex\"]\n[integrations.codex]\nskils_dir = \"/tmp/nope\"\n", "unknown override key"},
		{"none-location", "integrations = [\"generic\"]\n[integrations.generic]\nmemory_location = \"/tmp/nope\"\n", "requires memory_kind markdown-glob-v1"},
		{"unlisted", "integrations = [\"codex\"]\n[integrations.orphan]\nskills_dir = \"/tmp/nope\"\n", "not listed"},
		{"invalid-id", "integrations = [\"bad/id\"]\n", "must match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := writeConfig(t, tc.body)
			if _, err := adapters.Resolve(cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestIntegrationDuplicateIDsAreRejected(t *testing.T) {
	cfg := writeConfig(t, "integrations = [\"codex\", \"codex\"]\n")
	if _, err := adapters.Resolve(cfg); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("duplicate integration error = %v", err)
	}
}

func TestIntegrationConfigJSON(t *testing.T) {
	home := t.TempDir()
	cmd := exec.Command("go", "run", "./cmd/regesto", "--config", configFixture(t, "generic-profile.toml"), "config", "--json")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "HOME="+home, "REGESTO_MACHINE=fixture-box")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("config --json: %v\\nstderr: %s", err, stderr.String())
	}
	var got struct {
		LegacyAgents bool `json:"legacy_agents"`
		Integrations []struct {
			Name         string `json:"name"`
			ProfileID    string `json:"profile_id"`
			DefaultTrust string `json:"default_trust"`
		} `json:"integrations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.LegacyAgents || len(got.Integrations) != 1 || got.Integrations[0].Name != "synthetic" || got.Integrations[0].ProfileID != "generic" || got.Integrations[0].DefaultTrust != "quarantine" {
		t.Errorf("JSON = %s", stdout.String())
	}
}

func TestIntegrationConfigTextOutputUsesIntegrationVocabulary(t *testing.T) {
	home := t.TempDir()
	cmd := exec.Command("go", "run", "./cmd/regesto", "--config", configFixture(t, "generic-profile.toml"), "config")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "HOME="+home, "REGESTO_MACHINE=fixture-box")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("config: %v\\nstderr: %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"integrations=synthetic", "integration.synthetic.profile=generic", "integration.synthetic.trust=quarantine"} {
		if !strings.Contains(got, want) {
			t.Errorf("new config output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\nagents=") || strings.Contains(got, "\nagent.synthetic.") {
		t.Errorf("new config output mixed legacy vocabulary:\n%s", got)
	}
}

func TestIntegrationUpgradeUsesProfileAwareNewVocabulary(t *testing.T) {
	root, tools := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "adapters", "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version":1,"id":"custom-agent","display_name":"Custom","detect":{"commands":["custom-agent"]},"skills":{"targets":[],"variant":"portable"},"instructions":{"targets":[]},"hooks":[{"protocol":"none","registrar":"none"}],"memory":[{"kind":"none"}],"default_trust":"quarantine"}`
	if err := os.WriteFile(filepath.Join(root, "adapters", "profiles", "custom-agent.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("integrations = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tools, "custom-agent"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./cmd/regesto", "--config", filepath.Join(root, "config.toml"), "upgrade", "--dry-run")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "PATH="+tools+":"+os.Getenv("PATH"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("upgrade: %v\\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "custom-agent") || !strings.Contains(stdout.String(), "not in integrations = [...]") {
		t.Errorf("upgrade did not use profile-aware integration vocabulary:\n%s", stdout.String())
	}
}

func TestTrustUnknownIntegrationTargetContract(t *testing.T) {
	t.Skip("TODO(M3): unknown/custom integrations must default to quarantine")
}

// This pins parser/config compatibility only. The full zero-edit upgrade proof
// for a v0.3.1-style instance is intentionally deferred to milestone 11.
func TestIntegrationV031LegacyConfigFixture(t *testing.T) {
	cfg := loadConfigFixture(t, "legacy-v0.3.1.toml")
	if strings.Join(cfg.Agents, ",") != "claude,codex" {
		t.Fatalf("legacy agents = %v, want claude,codex", cfg.Agents)
	}
	if got := cfg.Projects["aurora-2"]; got != "aurora" {
		t.Errorf("legacy project mapping = %q, want aurora", got)
	}
}
