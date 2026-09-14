package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitIntegrationsFlagWritesConfig(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude,codex"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `integrations = ["claude", "codex"]`) {
		t.Fatalf("config.toml integrations line missing or wrong:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(root, ".state", "machine")); err != nil {
		t.Fatalf("machine identity not written: %v", err)
	}
}

func TestInitIntegrationsFlagRejectsUnknown(t *testing.T) {
	root := t.TempDir()
	err := runInit([]string{"--dir", root, "--integrations", "nosuch"})
	if err == nil {
		t.Fatal("unknown integration should fail init")
	}
	if _, statErr := os.Stat(filepath.Join(root, "config.toml")); statErr == nil {
		t.Fatal("nothing should be written after a rejected flag")
	}
}

func TestInitKeepsExistingConfigWithoutForce(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude"}); err != nil {
		t.Fatal(err)
	}
	if err := runInit([]string{"--dir", root, "--integrations", "codex"}); err == nil {
		t.Fatal("second init without --force should refuse")
	}
	body, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"claude"`) {
		t.Fatal("refused init must not have touched the existing config")
	}
}

func TestInitForceKeepsExistingConfigAndRejectsIntegrationsFlag(t *testing.T) {
	// With --force over an existing config the file is kept — the picker must
	// not run (its selection would be discarded) and --integrations, which
	// also cannot take effect, must be an error rather than silently ignored.
	root := t.TempDir()
	if err := runInit([]string{"--dir", root, "--integrations", "claude"}); err != nil {
		t.Fatal(err)
	}
	if err := runInit([]string{"--dir", root, "--force"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `integrations = ["claude"]`) {
		t.Fatalf("kept config must be the original one:\n%s", body)
	}
	err = runInit([]string{"--dir", root, "--force", "--integrations", "codex"})
	if err == nil || !strings.Contains(err.Error(), "--integrations has no effect") {
		t.Fatalf("--force with --integrations should error: %v", err)
	}
}
