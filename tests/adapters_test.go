// Tests that install locations come from config with vendor defaults, and that
// nothing personal is baked into code (PLAN §0 "two audiences", 4.b).
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regesto/internal/adapters"
	"regesto/internal/config"
)

func writeConfig(t *testing.T, body string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func agentByName(t *testing.T, list []adapters.Agent, name string) adapters.Agent {
	t.Helper()
	for _, a := range list {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("agent %q not returned", name)
	return adapters.Agent{}
}

func TestAdapterVendorDefaults(t *testing.T) {
	cfg := writeConfig(t, "machine = \"testbox\"\nagents = [\"claude\", \"codex\", \"hermes\"]\n")
	list := adapters.For(cfg)
	if len(list) != 3 {
		t.Fatalf("got %d agents, want 3", len(list))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	claude := agentByName(t, list, "claude")
	if want := filepath.Join(home, ".claude", "skills"); claude.SkillsDir != want {
		t.Errorf("claude skills = %q, want %q", claude.SkillsDir, want)
	}
	if want := filepath.Join(home, ".claude", "CLAUDE.md"); claude.InstructionsFile != want {
		t.Errorf("claude instructions = %q, want %q", claude.InstructionsFile, want)
	}
	if claude.SettingsFile == "" {
		t.Errorf("claude should have a settings file — it is the agent with hooks")
	}

	// Codex and Hermes have no hook mechanism, so no settings file. The
	// installer uses this to decide whether to try registering a hook.
	if a := agentByName(t, list, "codex"); a.SettingsFile != "" {
		t.Errorf("codex settings = %q, want empty", a.SettingsFile)
	}
	if a := agentByName(t, list, "hermes"); a.SettingsFile != "" {
		t.Errorf("hermes settings = %q, want empty", a.SettingsFile)
	}

	// No default may be a path belonging to one person.
	for _, a := range list {
		for _, p := range []string{a.SkillsDir, a.InstructionsFile, a.SettingsFile} {
			if p == "" {
				continue
			}
			if !strings.HasPrefix(p, home) {
				t.Errorf("%s default %q is not under the home directory", a.Name, p)
			}
			for _, personal := range []string{"dotfiles", "Workspace", "regesto-kb"} {
				if strings.Contains(p, personal) {
					t.Errorf("%s default %q hardcodes a personal path element %q", a.Name, p, personal)
				}
			}
		}
	}
}

func TestAdapterConfigOverridesWin(t *testing.T) {
	cfg := writeConfig(t, `machine = "testbox"
agents = ["claude", "codex"]

[skills_dirs]
claude = "~/.agents/skills"

[instructions]
claude = "~/.dotfiles/AGENTS.md"
codex = "~/.dotfiles/AGENTS.md"

[settings_files]
claude = "~/somewhere/settings.json"
`)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	list := adapters.For(cfg)

	claude := agentByName(t, list, "claude")
	if want := filepath.Join(home, ".agents", "skills"); claude.SkillsDir != want {
		t.Errorf("override ignored: skills = %q, want %q", claude.SkillsDir, want)
	}
	if want := filepath.Join(home, ".dotfiles", "AGENTS.md"); claude.InstructionsFile != want {
		t.Errorf("override ignored: instructions = %q, want %q", claude.InstructionsFile, want)
	}
	if want := filepath.Join(home, "somewhere", "settings.json"); claude.SettingsFile != want {
		t.Errorf("override ignored: settings = %q, want %q", claude.SettingsFile, want)
	}

	// An un-overridden agent keeps its vendor default.
	if want := filepath.Join(home, ".codex", "skills"); agentByName(t, list, "codex").SkillsDir != want {
		t.Errorf("codex skills should have stayed at the default %q", want)
	}
}

func TestUnknownAgentIsReportedNotSkipped(t *testing.T) {
	cfg := writeConfig(t, "machine = \"testbox\"\nagents = [\"claude\", \"someagent\"]\n")
	list := adapters.For(cfg)
	a := agentByName(t, list, "someagent")
	if a.SkillsDir != "" {
		t.Errorf("unknown agent got a guessed skills dir %q", a.SkillsDir)
	}
	// Returned rather than dropped, so the installer can warn instead of
	// silently doing nothing for an agent the user asked for.
	if len(list) != 2 {
		t.Errorf("got %d agents, want 2 including the unknown one", len(list))
	}
}

func TestArbitrarySectionsParseAndExpandHome(t *testing.T) {
	cfg := writeConfig(t, `machine = "testbox"
agents = ["claude"]

[projects]
"aurora-2" = "aurora"

[skills_dirs]
claude = "~/.agents/skills"

[future_thing]
key = "value"
`)
	if cfg.Projects["aurora-2"] != "aurora" {
		t.Errorf("projects section broke: %v", cfg.Projects)
	}
	if got := cfg.Section("future_thing")["key"]; got != "value" {
		t.Errorf("unknown section not parsed: got %q", got)
	}
	if got := cfg.Section("nope"); len(got) != 0 {
		t.Errorf("absent section should be an empty map, got %v", got)
	}
	home, _ := os.UserHomeDir()
	if got := cfg.Section("skills_dirs")["claude"]; !strings.HasPrefix(got, home) {
		t.Errorf("~ not expanded in section value: %q", got)
	}
}
