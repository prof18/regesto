package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/config"
	regestohooks "github.com/prof18/regesto/internal/hooks"
)

func hookFixture(t *testing.T, name, cwd string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("fixtures", "hooks", name))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.ReplaceAll(string(body), "{{cwd}}", cwd))
}

func hookConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, "config.toml", "machine = \"fixture\"\nintegrations = []\n")
	writeAt(t, root, "knowledge/facts/global.md", `---
schema_version: 1
id: hook-global
title: Hook fixture fact
type: context
scope: global
subject: hooks
relation: fixture
status: active
source: human
created: 2026-08-29T10:00:00Z
modified: 2026-08-29T10:00:00Z
---

Fixture.
`)
	cfg, err := config.Load(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, root
}

func runHookDirect(t *testing.T, cfg *config.Config, protocol string, payload []byte) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := regestohooks.Run(cfg, protocol, bytes.NewReader(payload), &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestClaudeHookPlainContextAndFailOpenFraming(t *testing.T) {
	cfg, cwd := hookConfig(t)
	prefix := string(hookFixture(t, "context-prefix.txt", cwd))
	for _, fixture := range []string{"claude-workspace.json", "claude-cwd.json"} {
		stdout, stderr, err := runHookDirect(t, cfg, "claude-session-start-v1", hookFixture(t, fixture, cwd))
		if err != nil || stderr != "" {
			t.Fatalf("%s: err=%v stderr=%q", fixture, err, stderr)
		}
		if !strings.HasPrefix(stdout, prefix) || !strings.Contains(stdout, "Hook fixture fact") || strings.HasPrefix(stdout, "{") {
			t.Fatalf("%s emitted wrong Claude framing:\n%s", fixture, stdout)
		}
	}
	stdout, stderr, err := runHookDirect(t, cfg, "claude-session-start-v1", hookFixture(t, "malformed.json", cwd))
	if err != nil || stdout != "" || stderr == "" {
		t.Fatalf("malformed Claude hook: stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
	stdout, _, err = runHookDirect(t, cfg, "claude-session-start-v1", []byte(`{"cwd":"`+cwd+`","cwd":"`+cwd+`"}`))
	if err != nil || stdout != "" {
		t.Fatalf("duplicate-key Claude hook: stdout=%q err=%v", stdout, err)
	}
	for name, payload := range map[string][]byte{
		"missing directories": []byte(`{}`),
		"unusable directory":  []byte(`{"cwd":"/path/that/does/not/exist"}`),
	} {
		stdout, stderr, err = runHookDirect(t, cfg, "claude-session-start-v1", payload)
		if err != nil || stdout != "" || stderr == "" {
			t.Fatalf("%s Claude hook: stdout=%q stderr=%q err=%v", name, stdout, stderr, err)
		}
	}
}

func TestHermesHookFirstTurnRepeatFallbackAndFailureFraming(t *testing.T) {
	cfg, cwd := hookConfig(t)
	first, stderr, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", hookFixture(t, "hermes-first.json", cwd))
	if err != nil || stderr != "" {
		t.Fatalf("Hermes first turn: err=%v stderr=%q", err, stderr)
	}
	var response struct {
		Context string `json:"context"`
	}
	if err := json.Unmarshal([]byte(first), &response); err != nil || !strings.Contains(response.Context, "Hook fixture fact") {
		t.Fatalf("Hermes first response = %q, err=%v", first, err)
	}
	empty := strings.TrimSpace(string(hookFixture(t, "hermes-empty-result.json", cwd)))
	repeatedFirst, _, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", hookFixture(t, "hermes-first.json", cwd))
	if err != nil || repeatedFirst != empty {
		t.Fatalf("Hermes repeated first-turn response = %q, err=%v", repeatedFirst, err)
	}
	next, _, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", hookFixture(t, "hermes-next.json", cwd))
	if err != nil || next != empty {
		t.Fatalf("Hermes repeat response = %q, err=%v", next, err)
	}
	falseBeforeTrue := []byte(`{"hook_event_name":"pre_llm_call","cwd":"` + cwd + `","session_id":"out-of-order","extra":{"is_first_turn":false}}`)
	if got, _, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", falseBeforeTrue); err != nil || got != empty {
		t.Fatalf("Hermes early false response = %q, err=%v", got, err)
	}
	trueAfterFalse := []byte(`{"hook_event_name":"pre_llm_call","cwd":"` + cwd + `","session_id":"out-of-order","extra":{"is_first_turn":true}}`)
	if got, _, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", trueAfterFalse); err != nil || got == empty {
		t.Fatalf("Hermes true after false response = %q, err=%v", got, err)
	}
	fallbackPayload := hookFixture(t, "hermes-marker.json", cwd)
	fallbackFirst, _, _ := runHookDirect(t, cfg, "hermes-pre-llm-v1", fallbackPayload)
	fallbackNext, _, _ := runHookDirect(t, cfg, "hermes-pre-llm-v1", fallbackPayload)
	if fallbackFirst == empty || fallbackNext != empty {
		t.Fatalf("Hermes marker responses: first=%q next=%q", fallbackFirst, fallbackNext)
	}
	malformed, diagnostic, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", hookFixture(t, "malformed.json", cwd))
	if err != nil || malformed != empty || diagnostic == "" {
		t.Fatalf("malformed Hermes hook: stdout=%q stderr=%q err=%v", malformed, diagnostic, err)
	}
}

func TestHermesContextFailureDoesNotConsumeSession(t *testing.T) {
	cfg, cwd := hookConfig(t)
	factPath := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global.md")
	validFact, err := os.ReadFile(factPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(factPath, []byte("not a fact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"hook_event_name":"pre_llm_call","cwd":"` + cwd + `","session_id":"retry-after-failure","extra":{"is_first_turn":true}}`)
	first, diagnostic, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", payload)
	if err != nil || first != "{}" || diagnostic == "" {
		t.Fatalf("failed Hermes context: stdout=%q diagnostic=%q err=%v", first, diagnostic, err)
	}
	if err := os.WriteFile(factPath, validFact, 0o644); err != nil {
		t.Fatal(err)
	}
	second, diagnostic, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", payload)
	if err != nil || diagnostic != "" || !strings.Contains(second, "Hook fixture fact") {
		t.Fatalf("retried Hermes context: stdout=%q diagnostic=%q err=%v", second, diagnostic, err)
	}
}

func TestHookPayloadBoundsAndWrongTypesFailOpen(t *testing.T) {
	cfg, cwd := hookConfig(t)
	oversized := bytes.Repeat([]byte("x"), (1<<20)+1)
	claude, diagnostic, err := runHookDirect(t, cfg, "claude-session-start-v1", oversized)
	if err != nil || claude != "" || diagnostic == "" {
		t.Fatalf("oversized Claude payload: stdout=%q diagnostic=%q err=%v", claude, diagnostic, err)
	}
	hermes, diagnostic, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", []byte(`{"cwd":42}`))
	if err != nil || hermes != "{}" || diagnostic == "" {
		t.Fatalf("wrong-type Hermes payload: stdout=%q diagnostic=%q err=%v", hermes, diagnostic, err)
	}
	otherEvent := []byte(`{"hook_event_name":"post_llm_call","cwd":"` + cwd + `","session_id":"other","extra":{}}`)
	hermes, diagnostic, err = runHookDirect(t, cfg, "hermes-pre-llm-v1", otherEvent)
	if err != nil || hermes != "{}" || diagnostic != "" {
		t.Fatalf("other Hermes event: stdout=%q diagnostic=%q err=%v", hermes, diagnostic, err)
	}
}

func TestKnownHookFailuresRemainHostValidAndUnknownProtocolErrors(t *testing.T) {
	cfg := &config.Config{KBRoot: filepath.Join(t.TempDir(), "missing"), Projects: map[string]string{}}
	validDir := t.TempDir()
	claude, _, err := runHookDirect(t, cfg, "claude-session-start-v1", []byte(`{"cwd":"`+validDir+`"}`))
	if err != nil || claude != "" {
		t.Fatalf("Claude operational failure: stdout=%q err=%v", claude, err)
	}
	hermes, _, err := runHookDirect(t, cfg, "hermes-pre-llm-v1", []byte(`{"cwd":"`+validDir+`","session_id":"s","extra":{"is_first_turn":true}}`))
	if err != nil || hermes != "{}" {
		t.Fatalf("Hermes operational failure: stdout=%q err=%v", hermes, err)
	}
	unknown, _, err := runHookDirect(t, cfg, "not-a-protocol", []byte(`{}`))
	if err == nil || unknown != "" {
		t.Fatalf("unknown protocol: stdout=%q err=%v", unknown, err)
	}
}

func TestHookCLIUsesExactProtocolFraming(t *testing.T) {
	cfg, cwd := hookConfig(t)
	cmd := exec.Command("go", "run", "./cmd/regesto", "--config", cfg.Path, "hook", "hermes-pre-llm-v1")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "GOTELEMETRY=off", "GOCACHE="+t.TempDir(), "GOPATH="+t.TempDir())
	cmd.Stdin = bytes.NewReader(hookFixture(t, "hermes-first.json", cwd))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("hook CLI: %v\nstderr=%s", err, stderr.String())
	}
	var response map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil || response["context"] == "" {
		t.Fatalf("hook CLI response=%q err=%v", stdout.String(), err)
	}
}

func TestHookCLIConfigFailureStillUsesHostValidFraming(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/regesto", "--config", filepath.Join(t.TempDir(), "missing.toml"), "hook", "hermes-pre-llm-v1")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "GOTELEMETRY=off", "GOCACHE="+t.TempDir(), "GOPATH="+t.TempDir())
	cmd.Stdin = strings.NewReader(`{"cwd":"/tmp","session_id":"s","extra":{"is_first_turn":true}}`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil || stdout.String() != "{}" || stderr.Len() == 0 {
		t.Fatalf("config-failure hook CLI: stdout=%q stderr=%q err=%v", stdout.String(), stderr.String(), err)
	}
}

func TestHookCompatibilityWrappersForwardAndFailOpen(t *testing.T) {
	files, err := regesto.InstanceFiles()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, relative := range []string{"adapters/claude/hooks/session-start.sh", "adapters/hermes/hooks/pre-llm.sh"} {
		writeAt(t, root, relative, string(files[relative]))
		if err := os.Chmod(filepath.Join(root, filepath.FromSlash(relative)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	engine := filepath.Join(root, "bin", "regesto-hook")
	writeAt(t, root, "bin/regesto-hook", "#!/bin/sh\ncat >/dev/null\nprintf 'plain-context\\n'\n")
	if err := os.Chmod(engine, 0o755); err != nil {
		t.Fatal(err)
	}
	claude := exec.Command(filepath.Join(root, "adapters", "claude", "hooks", "session-start.sh"))
	claude.Stdin = strings.NewReader("{malformed")
	if output, err := claude.CombinedOutput(); err != nil || string(output) != "plain-context\n" {
		t.Fatalf("Claude wrapper: output=%q err=%v", output, err)
	}
	writeAt(t, root, "bin/regesto-hook", "#!/bin/sh\ncat >/dev/null\nexit 42\n")
	if err := os.Chmod(engine, 0o755); err != nil {
		t.Fatal(err)
	}
	hermes := exec.Command(filepath.Join(root, "adapters", "hermes", "hooks", "pre-llm.sh"))
	hermes.Stdin = strings.NewReader("{malformed")
	if output, err := hermes.CombinedOutput(); err != nil || string(output) != "{}" {
		t.Fatalf("Hermes wrapper fail-open: output=%q err=%v", output, err)
	}
}

func TestHookShimHonorsKnowledgeBaseRootOverride(t *testing.T) {
	files, err := regesto.InstanceFiles()
	if err != nil {
		t.Fatal(err)
	}
	launcherRoot, instanceRoot := t.TempDir(), t.TempDir()
	shim := filepath.Join(launcherRoot, "bin", "regesto-hook")
	writeAt(t, launcherRoot, "bin/regesto-hook", string(files["bin/regesto-hook"]))
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	engine := filepath.Join(instanceRoot, "bin", "regesto")
	writeAt(t, instanceRoot, "bin/regesto", "#!/bin/sh\nprintf '%s\\n' \"$@\"\n")
	if err := os.Chmod(engine, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(shim, "hermes-pre-llm-v1")
	cmd.Env = append(os.Environ(), "REGESTO_KB_ROOT="+instanceRoot)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := "--config\n" + filepath.Join(instanceRoot, "config.toml") + "\nhook\nhermes-pre-llm-v1\n"
	if string(output) != want {
		t.Fatalf("override hook args = %q, want %q", output, want)
	}
}
