package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/config"
	regestoinstall "github.com/prof18/regesto/internal/install"
)

func skillTestConfig(t *testing.T, root, body string) *config.Config {
	t.Helper()
	materializeInstallHook(t, root)
	path := filepath.Join(root, "config.toml")
	writeAt(t, root, "config.toml", body)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestPortableSkillSourcesUseOnlyPortableContract(t *testing.T) {
	files, err := regesto.InstanceFiles()
	if err != nil {
		t.Fatal(err)
	}
	for path, body := range files {
		if !strings.HasPrefix(path, "adapters/skills/") || filepath.Base(path) != "SKILL.md" {
			continue
		}
		text := string(body)
		lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		if len(lines) < 4 || lines[0] != "---" {
			t.Fatalf("%s lacks frontmatter", path)
		}
		closing := -1
		fields := map[string]bool{}
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				closing = i
				break
			}
			if strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(lines[i], " ") {
				continue
			}
			key, _, ok := strings.Cut(lines[i], ":")
			if !ok {
				t.Fatalf("%s malformed frontmatter line %q", path, lines[i])
			}
			fields[key] = true
		}
		if closing < 0 || !fields["name"] || !fields["description"] {
			t.Fatalf("%s missing required portable fields: %v", path, fields)
		}
		for field := range fields {
			if field != "name" && field != "description" && field != "license" && field != "compatibility" && field != "metadata" {
				t.Fatalf("%s has non-portable frontmatter field %q", path, field)
			}
		}
		assertNoPortableSkillLeak(t, path, text)
	}
	assertNoPortableSkillLeak(t, "adapters/instructions/regesto-section.md", string(files["adapters/instructions/regesto-section.md"]))
}

func assertNoPortableSkillLeak(t *testing.T, path, text string) {
	t.Helper()
	for _, forbidden := range []string{"Claude Code", "Codex", "Hermes", "$ARGUMENTS", "!`"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("%s leaks %q", path, forbidden)
		}
	}
}

func TestBuiltInIntegrationsRenderIdenticalPortableSkills(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".gemini/antigravity-cli", ".claude", ".codex", ".copilot", ".hermes"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := skillTestConfig(t, root, "integrations = [\"antigravity\", \"claude\", \"codex\", \"copilot\", \"hermes\"]\n")
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := regestoinstall.Apply(plan); err != nil {
		t.Fatal(err)
	}
	readRendered := func(id string) []byte {
		t.Helper()
		return mustRead(t, filepath.Join(root, ".state", "integrations", id, "skills", "regesto-search", "SKILL.md"))
	}
	antigravity, claude, codex, copilot, hermes := readRendered("antigravity"), readRendered("claude"), readRendered("codex"), readRendered("copilot"), readRendered("hermes")
	if !bytes.Equal(claude, codex) {
		t.Fatalf("claude render differs from portable:\nClaude:\n%s\nCodex:\n%s", claude, codex)
	}
	if !bytes.Equal(antigravity, codex) || !bytes.Equal(codex, copilot) || !bytes.Equal(codex, hermes) {
		t.Fatalf("portable variants differ:\nAntigravity:\n%s\nCodex:\n%s\nCopilot:\n%s\nHermes:\n%s", antigravity, codex, copilot, hermes)
	}
	assertNoPortableSkillLeak(t, "antigravity rendered search", string(antigravity))
	assertNoPortableSkillLeak(t, "claude rendered search", string(claude))
	assertNoPortableSkillLeak(t, "codex rendered search", string(codex))
	assertNoPortableSkillLeak(t, "copilot rendered search", string(copilot))
	assertNoPortableSkillLeak(t, "hermes rendered search", string(hermes))
	for id, target := range map[string]string{"antigravity": ".gemini/antigravity-cli", "claude": ".claude", "codex": ".codex", "copilot": ".copilot", "hermes": ".hermes"} {
		link := filepath.Join(home, target, "skills", "regesto-search")
		got, err := os.Readlink(link)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(root, ".state", "integrations", id, "skills", "regesto-search")
		if got != want {
			t.Fatalf("%s skill link = %q, want %q", id, got, want)
		}
	}
	second, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("second variant install has %d changes", second.Changes())
	}
}

func TestCustomIntegrationRendersPortableTreeWithoutBuiltInName(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	skills := filepath.Join(home, ".synthetic", "skills")
	instructions := filepath.Join(home, ".synthetic", "INSTRUCTIONS.md")
	if err := os.MkdirAll(filepath.Dir(instructions), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instructions, []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "integrations = [\"synthetic\"]\n[integrations.synthetic]\nprofile = \"generic\"\nskills_dir = \"" + skills + "\"\ninstructions_file = \"" + instructions + "\"\n"
	cfg := skillTestConfig(t, root, body)
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := regestoinstall.Apply(plan); err != nil {
		t.Fatal(err)
	}
	rendered := mustRead(t, filepath.Join(root, ".state", "integrations", "synthetic", "skills", "regesto-write", "SKILL.md"))
	assertNoPortableSkillLeak(t, "synthetic rendered write", string(rendered))
	if got, err := os.Readlink(filepath.Join(skills, "regesto-write")); err != nil || !strings.Contains(got, filepath.Join("integrations", "synthetic", "skills")) {
		t.Fatalf("synthetic skill link = %q, err=%v", got, err)
	}
	if got := string(mustRead(t, instructions)); !strings.Contains(got, "foreign") || !strings.Contains(got, "regesto:section:start") {
		t.Fatalf("synthetic instructions not merged:\n%s", got)
	}
}

func writeDemoVariant(t *testing.T, root, protocols string) {
	t.Helper()
	writeAt(t, root, "adapters/variants/demo/variant.json", `{"schema_version":1,"id":"demo","mode":"append","requires_hook_protocols":[`+protocols+`]}`)
	writeAt(t, root, "adapters/variants/demo/skills/regesto-search/SKILL.md", "Demo appendix.\n")
	writeAt(t, root, "adapters/profiles/demo.json", `{"schema_version":1,"id":"demo","display_name":"Demo","skills":{"targets":["~/.demo/skills"],"variant":"demo"},"instructions":{"targets":[]},"hooks":[{"protocol":"none","registrar":"none"}],"memory":[{"kind":"none"}],"default_trust":"quarantine"}`)
}

func TestInstanceVariantAppendsOnlyToItsIntegration(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	writeDemoVariant(t, root, "")
	body := "integrations = [\"demo\", \"synthetic\"]\n" +
		"[integrations.synthetic]\nprofile = \"generic\"\nskills_dir = \"" + filepath.Join(home, ".synthetic", "skills") + "\"\n"
	cfg := skillTestConfig(t, root, body)
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := regestoinstall.Apply(plan); err != nil {
		t.Fatal(err)
	}
	demo := string(mustRead(t, filepath.Join(root, ".state", "integrations", "demo", "skills", "regesto-search", "SKILL.md")))
	synthetic := string(mustRead(t, filepath.Join(root, ".state", "integrations", "synthetic", "skills", "regesto-search", "SKILL.md")))
	if !strings.Contains(demo, "Demo appendix.") {
		t.Fatalf("demo render lacks its variant appendix:\n%s", demo)
	}
	if strings.Contains(synthetic, "Demo appendix.") {
		t.Fatalf("variant leaked into synthetic render:\n%s", synthetic)
	}
}

func TestSharedSkillsTargetRejectsDifferentVariants(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	shared := filepath.Join(home, "shared-skills")
	writeDemoVariant(t, root, "")
	body := "integrations = [\"demo\", \"portable\"]\n" +
		"[integrations.demo]\nskills_dir = \"" + shared + "\"\n" +
		"[integrations.portable]\nprofile = \"generic\"\nskills_dir = \"" + shared + "\"\n"
	cfg := skillTestConfig(t, root, body)
	_, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err == nil || !strings.Contains(err.Error(), "render variants") {
		t.Fatalf("shared variant conflict error = %v", err)
	}
}

func TestVariantRequiresDeclaredHookProtocol(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	writeDemoVariant(t, root, `"claude-session-start-v1"`)
	cfg := skillTestConfig(t, root, "integrations = [\"demo\"]\n")
	_, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err == nil || !strings.Contains(err.Error(), "requires undeclared hook protocol") {
		t.Fatalf("variant protocol error = %v", err)
	}
}

func TestUnknownOrInvalidVariantFailsClosed(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	profile := `{"schema_version":1,"id":"unknown-variant","display_name":"Unknown variant","skills":{"targets":["~/.unknown/skills"],"variant":"missing"},"instructions":{"targets":[]},"hooks":[{"protocol":"none","registrar":"none"}],"memory":[{"kind":"none"}],"default_trust":"quarantine"}`
	writeAt(t, root, "adapters/profiles/unknown-variant.json", profile)
	cfg := skillTestConfig(t, root, "integrations = [\"unknown-variant\"]\n")
	_, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err == nil || !strings.Contains(err.Error(), "unknown skills variant") {
		t.Fatalf("unknown variant error = %v", err)
	}
}
