package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
)

func loadTestConfig(t *testing.T, root string) *config.Config {
	t.Helper()
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestIntegrationsAddAppendsAndPreservesComments(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "# Regesto instance configuration.") {
		t.Fatal("template comment missing before the edit")
	}

	if err := runIntegrations(loadTestConfig(t, root), []string{"add", "codex", "hermes"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), `integrations = ["claude", "codex", "hermes"]`) {
		t.Fatalf("list not appended:\n%s", after)
	}
	if !strings.Contains(string(after), "# Regesto instance configuration.") {
		t.Fatalf("template comments lost:\n%s", after)
	}
	// The whole file beyond the one line must be byte-identical.
	oldLines, newLines := strings.Split(string(before), "\n"), strings.Split(string(after), "\n")
	if len(oldLines) != len(newLines) {
		t.Fatalf("line count changed: %d → %d", len(oldLines), len(newLines))
	}
	diffs := 0
	for i := range oldLines {
		if oldLines[i] != newLines[i] {
			diffs++
		}
	}
	if diffs != 1 {
		t.Fatalf("expected exactly one changed line, got %d", diffs)
	}
}

func TestIntegrationsAddIsNoOpForPresent(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude,codex"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := runIntegrations(loadTestConfig(t, root), []string{"add", "codex"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("no-op add rewrote the config")
	}
}

func TestIntegrationsAddRejectsUnknownAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	err = runIntegrations(loadTestConfig(t, root), []string{"add", "nosuch"})
	if err == nil || !strings.Contains(err.Error(), "known:") {
		t.Fatalf("unknown id should be rejected with the known list: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(root, config.FileName))
	if string(before) != string(after) {
		t.Fatal("rejected add must not write")
	}
}

func TestIntegrationsNoIdsWithoutTerminalIsAnError(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude"}); err != nil {
		t.Fatal(err)
	}
	// go test has no usable terminal, so the interactive setter must decline
	// with guidance instead of hanging or silently rewriting.
	err := runIntegrations(loadTestConfig(t, root), nil)
	if err == nil || !strings.Contains(err.Error(), "pass ids") {
		t.Fatalf("no-terminal no-ids should error with guidance: %v", err)
	}
}

func TestIntegrationsRemoveDropsOverrideSectionAndStaysLoadable(t *testing.T) {
	// The brick repro: a config with an [integrations.<id>] override for an id
	// that then leaves the list. Removal must drop the section too, or
	// adapters.Resolve rejects the config and every command — including this
	// one — fails to load the instance at all.
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude,codex"}); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, config.FileName)
	f, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("[integrations.codex]\ntrust = \"supervised\"\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := loadConfig(cfgPath); err != nil {
		t.Fatalf("setup: override config must load: %v", err)
	}

	if err := runIntegrations(loadTestConfig(t, root), []string{"remove", "codex"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "[integrations.codex]") || strings.Contains(string(body), `trust = "supervised"`) {
		t.Fatalf("override section not dropped:\n%s", body)
	}
	if !strings.Contains(string(body), `integrations = ["claude"]`) {
		t.Fatalf("codex not removed from the list:\n%s", body)
	}
	if _, err := loadConfig(cfgPath); err != nil {
		t.Fatalf("instance no longer loads — the brick: %v", err)
	}
}

func TestIntegrationsRemoveNotConfiguredIsNoOp(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := runIntegrations(loadTestConfig(t, root), []string{"remove", "codex"}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(root, config.FileName))
	if string(before) != string(after) {
		t.Fatal("removing a not-configured id rewrote the config")
	}
}

func TestIntegrationsBareIdsDefaultToAdd(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude"}); err != nil {
		t.Fatal(err)
	}
	// "codex" is a known id, not a subcommand: it must be treated as an add.
	if err := runIntegrations(loadTestConfig(t, root), []string{"codex"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `integrations = ["claude", "codex"]`) {
		t.Fatalf("bare id not added:\n%s", body)
	}
}

func TestIntegrationsUnknownIdIsRejected(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude"}); err != nil {
		t.Fatal(err)
	}
	// A typo'd subcommand becomes a first-class unknown-id error listing the
	// known integrations.
	err := runIntegrations(loadTestConfig(t, root), []string{"frobnicate"})
	if err == nil || !strings.Contains(err.Error(), "known:") {
		t.Fatalf("unknown id should be rejected with the known list: %v", err)
	}
}
