package tests

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/adapters"
)

func TestDocsCapabilityMatrixMatchesProfileMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	profiles, err := adapters.Profiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "agent-integration.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	start := strings.Index(text, "<!-- regesto:profile-matrix:start -->")
	end := strings.Index(text, "<!-- regesto:profile-matrix:end -->")
	if start < 0 || end <= start {
		t.Fatal("canonical profile matrix markers are missing or reversed")
	}
	matrix := text[start:end]
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	wantRows := make([]string, 0, len(ids))
	for _, id := range ids {
		wantRows = append(wantRows, profileMatrixRow(profiles[id]))
	}
	var gotRows []string
	for _, line := range strings.Split(matrix, "\n") {
		if strings.HasPrefix(line, "| `") {
			gotRows = append(gotRows, line)
		}
	}
	if strings.Join(gotRows, "\n") != strings.Join(wantRows, "\n") {
		t.Errorf("profile matrix row set/order is stale\ngot:\n%s\nwant:\n%s", strings.Join(gotRows, "\n"), strings.Join(wantRows, "\n"))
	}
}

func profileMatrixRow(profile adapters.Profile) string {
	detection := "paths=" + codeList(profile.Detect.Paths) + "; commands=" + codeList(profile.Detect.Commands)
	skills := "variant=" + codeValue(profile.Skills.Variant) + "; targets=" + codeList(profile.Skills.Targets)
	instructions := "targets=" + codeList(profile.Instructions.Targets) + "; create=" + codeValue(fmt.Sprint(profile.Instructions.Create))
	hooks := "none"
	if len(profile.Hooks) > 0 {
		parts := make([]string, 0, len(profile.Hooks))
		for _, hook := range profile.Hooks {
			part := codeValue(hook.Protocol) + " / " + codeValue(hook.Registrar)
			if hook.Settings != "" {
				part += " / " + codeValue(hook.Settings)
			}
			parts = append(parts, part)
		}
		hooks = strings.Join(parts, "<br>")
	}
	memory := "none"
	if len(profile.Memory) > 0 {
		parts := make([]string, 0, len(profile.Memory))
		for _, source := range profile.Memory {
			part := codeValue(source.Kind)
			if source.Location != "" {
				part += " / " + codeValue(source.Location)
			}
			parts = append(parts, part)
		}
		memory = strings.Join(parts, "<br>")
	}
	exclusions := codeList(profile.ExcludeGlobs)
	return fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %s |",
		codeValue(profile.ID), profile.DisplayName, detection, skills, instructions, hooks, memory, exclusions, codeValue(profile.DefaultTrust))
}

func codeList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = codeValue(value)
	}
	return strings.Join(quoted, ", ")
}

func codeValue(value string) string {
	return "`" + value + "`"
}

func TestDocsProductPagesLinkCanonicalMatrix(t *testing.T) {
	for _, name := range []string{"setup-claude-code.md", "setup-codex.md", "setup-hermes.md", "setup-other-agents.md"} {
		body, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "(agent-integration.md)") {
			t.Errorf("%s does not link the canonical agent integration matrix", name)
		}
	}
}

func TestDocsSetupGuideCoversEveryConnectionPath(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "setup-other-agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"## Built-in integrations",
		"## Custom local integration",
		"## MCP client",
		"## No local integration surface",
		`profile = "generic"`,
		"does not need an entry in `integrations`",
		`"args": ["--config", "/Users/you/regesto-kb/config.toml", "mcp"]`,
		"regesto promote ~/Downloads/conversation.md",
		"[normalize]",
		`--command "claude -p"`,
		"regesto doctor --integration my-agent",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("connection guide lacks %q", want)
		}
	}
}

func TestDocsCanonicalExamplesUseMatchingOverrideVocabulary(t *testing.T) {
	checks := map[string][]string{
		"setup-claude-code.md": {"[integrations.claude]", "skills_dir =", "instructions_file =", "settings_file ="},
		"setup-codex.md":       {"[integrations.codex]", "skills_dir =", "--config ~/regesto-kb/config.toml config", "--config ~/regesto-kb/config.toml install --dry-run"},
	}
	for name, wants := range checks {
		body, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing canonical override %q", name, want)
			}
		}
		for _, legacy := range []string{"[skills_dirs]", "[instructions]", "[settings_files]"} {
			if strings.Contains(text, legacy) {
				t.Errorf("%s mixes integrations with legacy override table %s", name, legacy)
			}
		}
	}
}

func TestDocsUnknownHostRecipeIsStandalone(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "agent-integration.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		`integrations = ["my-agent"]`, `[integrations.my-agent]`, `profile = "generic"`,
		`skills_dir =`, `instructions_file =`, `install --dry-run`,
		`doctor --integration my-agent`, `regesto --config ~/regesto-kb/config.toml mcp`,
		"change its `id` to the filename ID",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("standalone unknown-host recipe lacks %q", want)
		}
	}
}

func TestDocsInitEmitsCanonicalIntegrationVocabulary(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "kb")
	cmd := exec.Command("go", "run", "./cmd/regesto", "init", "--dir", root, "--machine", "docsbox")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "HOME="+home, "GOTELEMETRY=off", "GOCACHE="+t.TempDir(), "GOPATH="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("init: %v\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}
	body, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, `integrations = ["codex"]`) {
		t.Fatalf("init did not use canonical integration vocabulary:\n%s", text)
	}
	if strings.Contains(text, "\nagents = [") {
		t.Fatalf("init mixed legacy agents vocabulary into a new config:\n%s", text)
	}
}

func TestDocsInitWithoutDetectionKeepsCanonicalIntegrationVocabulary(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "kb")
	cmd := exec.Command("go", "run", "./cmd/regesto", "init", "--dir", root, "--machine", "emptybox")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH=/usr/bin:/bin", "GOTELEMETRY=off", "GOCACHE="+t.TempDir(), "GOPATH="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("init: %v\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}
	body, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "integrations = []") {
		t.Fatalf("no-detection init did not emit an active canonical list:\n%s", body)
	}
}
