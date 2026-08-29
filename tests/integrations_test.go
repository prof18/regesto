package tests

import (
	"bytes"
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
	body, err := os.ReadFile(configFixture(t, "generic-profile.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"integrations = [\"synthetic\"]", "profile = \"generic\"", "skills_dir = \"~/.synthetic/skills\"", "trust = \"quarantine\""} {
		if !strings.Contains(string(body), field) {
			t.Errorf("generic target fixture missing %q", field)
		}
	}
	t.Skip("TODO(M2): generic integrations profile/config resolution; fixture contract verified")
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
