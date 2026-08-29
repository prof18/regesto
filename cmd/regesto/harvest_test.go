package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
)

func harvestCommandConfig(t *testing.T, body string) *config.Config {
	t.Helper()
	t.Setenv("REGESTO_MACHINE", "testbox")
	root := t.TempDir()
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestHarvestOutputLabelsTypedSources(t *testing.T) {
	memory := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(memory, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := harvestCommandConfig(t, "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \""+memory+"\"\n")
	out, err := captureNormalizeStdout(t, func() error { return runHarvest(cfg, []string{"--dry-run", "-v"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "maker[markdown-glob-v1-") || !strings.Contains(out, "scanned 0, nothing new") || !strings.Contains(out, "dry run") {
		t.Fatalf("typed harvest output = %q", out)
	}
}

func TestHarvestOutputReportsNoneAndUnsupportedKinds(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "none", body: "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\n", want: "memory kind none"},
		{name: "unsupported", body: "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"sqlite-v1\"\n", want: "unsupported memory kind"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := harvestCommandConfig(t, test.body)
			out, err := captureNormalizeStdout(t, func() error { return runHarvest(cfg, nil) })
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "maker[") || !strings.Contains(out, test.want) {
				t.Fatalf("status output = %q", out)
			}
		})
	}
}
