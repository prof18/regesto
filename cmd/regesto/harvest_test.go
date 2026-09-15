package main

import (
	"flag"
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

func TestHarvestOutputLabelsSourcesPlainly(t *testing.T) {
	memory := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(memory, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := harvestCommandConfig(t, "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \""+memory+"\"\n")
	out, err := captureNormalizeStdout(t, func() error { return runHarvest(cfg, []string{"--dry-run", "-v"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "maker: nothing new (scanned 0 files)") || !strings.Contains(out, "Dry run") || strings.Contains(out, "markdown-glob-v1-") {
		t.Fatalf("typed harvest output = %q", out)
	}
}

func TestHarvestOutputReportsNoneAndUnsupportedKinds(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "none", body: "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\n", want: "has no memory files to collect"},
		{name: "unsupported", body: "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"sqlite-v1\"\n", want: "memory type \"sqlite-v1\" isn't supported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := harvestCommandConfig(t, test.body)
			out, err := captureNormalizeStdout(t, func() error { return runHarvest(cfg, nil) })
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "maker: ") || !strings.Contains(out, test.want) {
				t.Fatalf("status output = %q", out)
			}
		})
	}
}

func backfillCommandConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	memory := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(memory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.md", "two.md"} {
		if err := os.WriteFile(filepath.Join(memory, name), []byte("from before regesto"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := harvestCommandConfig(t, "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \""+memory+"\"\n")
	return cfg, memory
}

func inboxCount(t *testing.T, cfg *config.Config) int {
	t.Helper()
	n := 0
	_ = filepath.Walk(filepath.Join(cfg.KBRoot, "inbox"), func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestHarvestBackfillFlagImportsExistingMemory(t *testing.T) {
	cfg, _ := backfillCommandConfig(t)
	out, err := captureNormalizeStdout(t, func() error { return runHarvest(cfg, []string{"maker", "--backfill"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "importing 2 existing file(s)") || !strings.Contains(out, "2 notes added to the inbox") {
		t.Fatalf("backfill output = %q", out)
	}
	if got := inboxCount(t, cfg); got != 2 {
		t.Fatalf("inbox has %d files, want 2", got)
	}
}

func TestHarvestAgentNamesNeedBackfill(t *testing.T) {
	cfg, _ := backfillCommandConfig(t)
	if err := runHarvest(cfg, []string{"maker"}); err == nil || !strings.Contains(err.Error(), "--backfill") {
		t.Fatalf("err = %v, want a pointer to --backfill", err)
	}
}

func TestOfferMemoryImport(t *testing.T) {
	for _, test := range []struct {
		answer    string
		wantCount int
		wantText  string
	}{
		{answer: "y\n", wantCount: 2, wantText: "Imported 2 notes into the inbox"},
		{answer: "\n", wantCount: 0, wantText: "harvest --backfill"},
	} {
		t.Run(strings.TrimSpace(test.answer), func(t *testing.T) {
			cfg, _ := backfillCommandConfig(t)
			var out strings.Builder
			got := offerMemoryImport(strings.NewReader(test.answer), &out, cfg.Path)
			if got != test.wantCount || inboxCount(t, cfg) != test.wantCount {
				t.Fatalf("imported %d, inbox %d, want %d", got, inboxCount(t, cfg), test.wantCount)
			}
			if !strings.Contains(out.String(), "2 files") || !strings.Contains(out.String(), test.wantText) {
				t.Fatalf("prompt output = %q", out.String())
			}
		})
	}
}

func TestFlagsFirstKeepsValuesWithTheirFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Bool("backfill", false, "")
	fs.String("name", "", "")
	got := flagsFirst(fs, []string{"codex", "--name", "x", "--backfill", "claude", "--name=y"})
	want := []string{"--name", "x", "--backfill", "--name=y", "codex", "claude"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("flagsFirst = %v, want %v", got, want)
	}
}
