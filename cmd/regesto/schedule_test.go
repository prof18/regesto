package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
)

func TestSchedulePathIncludesConfiguredCommandAndStableDefaults(t *testing.T) {
	home := t.TempDir()
	toolDir := filepath.Join(home, "agent-tools")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(toolDir, "claude")
	if err := os.WriteFile(claude, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", toolDir)

	cfg := &config.Config{Sections: map[string]map[string]string{
		"normalize": {"commands": "claude -p"},
	}}
	got := filepath.SplitList(schedulePath(cfg, filepath.Join(home, "engine", "regesto")))

	wants := []string{
		filepath.Join(home, "engine"),
		toolDir,
		filepath.Join(home, ".local", "bin"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
	}
	for _, want := range wants {
		if !contains(got, want) {
			t.Errorf("scheduled PATH does not contain %q: %v", want, got)
		}
	}
}

func TestIntegrationInitTemplateKeepsLegacyAgentsAndDocumentsGenericProfile(t *testing.T) {
	body := instanceConfig([]string{"claude", "codex"})
	for _, want := range []string{
		"agents = [\"claude\", \"codex\"]",
		"Replace or remove the generated agents = [...] line above first",
		"# integrations = [\"my-agent\"]",
		"[integrations.my-agent]",
		"memory_kind = \"markdown-glob-v1\"",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("init template missing %q", want)
		}
	}
	if strings.Contains(body, "\nintegrations = [") {
		t.Error("init template must not create integrations alongside legacy agents")
	}
}

func TestScheduleExtraPathComesFirstAndIsDeduplicated(t *testing.T) {
	cfg := &config.Config{Sections: map[string]map[string]string{
		"schedule": {"extra_path": "/custom/bin:/opt/homebrew/bin:/custom/bin"},
	}}
	got := filepath.SplitList(schedulePath(cfg, "/stable/bin/regesto"))

	if got[0] != "/custom/bin" {
		t.Fatalf("extra path should take precedence, got %v", got)
	}
	if count(got, "/custom/bin") != 1 || count(got, "/opt/homebrew/bin") != 1 {
		t.Fatalf("scheduled PATH should be deduplicated, got %v", got)
	}
}

func TestPlistIncludesEscapedScheduledPath(t *testing.T) {
	cfg := &config.Config{
		KBRoot:  "/tmp/kb & notes",
		Machine: "studio",
		Sections: map[string]map[string]string{
			"schedule": {"extra_path": "/tools & agents/bin"},
		},
	}
	body := plist(cfg, job{label: "com.regesto.cycle", args: []string{"cycle"}, interval: 3600}, "/stable/bin/regesto")

	for _, want := range []string{
		"<key>EnvironmentVariables</key>",
		"<key>PATH</key>",
		"/tools &amp; agents/bin",
		"/tmp/kb &amp; notes/config.toml",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("plist does not contain %q:\n%s", want, body)
		}
	}
}

func contains(values []string, target string) bool { return count(values, target) > 0 }

func count(values []string, target string) int {
	total := 0
	for _, value := range values {
		if value == target {
			total++
		}
	}
	return total
}
