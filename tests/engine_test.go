// An instance is knowledge; the engine is a binary that may live anywhere. These
// pin the resolution order, because getting it wrong fails in the worst possible
// place: a launchd job holding a path that no longer runs, erroring every fire
// into a log nobody reads.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
)

func engineInstance(t *testing.T) *config.Config {
	t.Helper()
	return writeInstance(t, "agents = [\"claude\"]\nmachine = \"testbox\"\n", "")
}

func writeBinary(t *testing.T, cfg *config.Config, mode os.FileMode) string {
	t.Helper()
	dir := filepath.Join(cfg.KBRoot, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "regesto")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// A checkout rebuilds bin/regesto in place, so anything holding that path picks
// up the new build without being reinstalled. It has to win.
func TestEngineInsideTheInstanceWins(t *testing.T) {
	cfg := engineInstance(t)
	want := writeBinary(t, cfg, 0o755)
	got, err := config.ResolveEngine(cfg)
	if err != nil {
		t.Fatalf("ResolveEngine: %v", err)
	}
	if got != want {
		t.Errorf("engine = %q, want the instance's own %q", got, want)
	}
}

// A non-executable file at that path is not an engine. Treating it as one would
// schedule a job that can never run.
func TestNonExecutableInstanceBinaryIsNotTheEngine(t *testing.T) {
	cfg := engineInstance(t)
	skipped := writeBinary(t, cfg, 0o644)
	got, _ := config.ResolveEngine(cfg)
	if got == skipped {
		t.Errorf("a non-executable %q was accepted as the engine", skipped)
	}
}

// With no binary in the instance, resolution falls back to the running
// executable — which under `go test` is a temporary build. That is exactly the
// case that must be refused rather than recorded, and asserting it here is what
// proves the guard fires at all.
func TestTemporaryBuildIsRefused(t *testing.T) {
	cfg := engineInstance(t)
	got, err := config.ResolveEngine(cfg)
	if err == nil {
		t.Fatalf("a temporary build at %q was accepted as a schedulable engine", got)
	}
	if !strings.Contains(err.Error(), "deleted on exit") {
		t.Errorf("error should explain why the path is unusable, got: %v", err)
	}
	// It must also say how to get out of the situation, not just refuse.
	if !strings.Contains(err.Error(), "go build") {
		t.Errorf("error should name a remedy, got: %v", err)
	}
}

// The instance never needs to hold a binary: an engine on PATH serves it just as
// well, which is what lets the knowledge base hold only knowledge. The resolver
// must not require KBRoot/bin to exist at all.
func TestMissingInstanceBinIsNotAnError(t *testing.T) {
	cfg := engineInstance(t)
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "bin")); err == nil {
		t.Fatal("this instance was supposed to have no bin/ directory")
	}
	_, err := config.ResolveEngine(cfg)
	if err != nil && !strings.Contains(err.Error(), "deleted on exit") {
		t.Errorf("a missing bin/ should fall through, not fail: %v", err)
	}
}
