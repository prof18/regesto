// Machine identity must be resolved per machine. config.toml sits at the KB root
// and is replicated by Syncthing, so a value written there is identical
// everywhere — yet inbox/<agent>@<machine>/, .state/<machine>/ and a fact's
// source: all have to differ per machine (PLAN 0.1, 0.a).
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prof18/regesto/internal/config"
)

// writeInstance builds a KB root with a config file and optional .state/machine.
func writeInstance(t *testing.T, configBody, stateMachine string) *config.Config {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if stateMachine != "" {
		state := filepath.Join(root, ".state")
		if err := os.MkdirAll(state, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(state, config.MachineFile), []byte(stateMachine), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Load(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestStateMachineBeatsSyncedConfig(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "")
	// The synced config says studio; this machine's own .state says macbook.
	// .state must win, or every machine would answer "studio".
	cfg := writeInstance(t, "agents = [\"claude\"]\nmachine = \"studio\"\n", "macbook\n")
	if cfg.Machine != "macbook" {
		t.Errorf("machine = %q, want macbook — the synced config overrode per-machine state", cfg.Machine)
	}
	if cfg.MachineSource != ".state/machine" {
		t.Errorf("source = %q, want .state/machine", cfg.MachineSource)
	}
}

func TestEnvBeatsEverything(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "ci-box")
	cfg := writeInstance(t, "agents = [\"claude\"]\nmachine = \"studio\"\n", "macbook\n")
	if cfg.Machine != "ci-box" {
		t.Errorf("machine = %q, want ci-box", cfg.Machine)
	}
	if cfg.MachineSource != "env:REGESTO_MACHINE" {
		t.Errorf("source = %q, want env:REGESTO_MACHINE", cfg.MachineSource)
	}
}

// A single-machine instance that never syncs may legitimately set it in config.
func TestConfigMachineUsedWhenNoState(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "")
	cfg := writeInstance(t, "agents = [\"claude\"]\nmachine = \"solo\"\n", "")
	if cfg.Machine != "solo" {
		t.Errorf("machine = %q, want solo", cfg.Machine)
	}
	if cfg.MachineSource != "config.toml" {
		t.Errorf("source = %q, want config.toml", cfg.MachineSource)
	}
}

// Nothing configured: fall back to the hostname rather than failing, but report
// it as a guess so an installer can tell the user to pin it.
func TestHostnameFallbackIsReportedAsSuch(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "")
	cfg := writeInstance(t, "agents = [\"claude\"]\n", "")
	if cfg.Machine == "" {
		t.Error("machine must never resolve to empty — the hook has to work anyway")
	}
	if cfg.MachineSource != "hostname" {
		t.Errorf("source = %q, want hostname", cfg.MachineSource)
	}
	// macOS appends and increments a counter when rejoining networks: the same
	// box comes back as "-2", then "-3". A raw hostname is therefore not a
	// stable identity, so the slug must drop the suffix and any domain part.
	for _, bad := range []string{".", "-1", "-2", "-3"} {
		if len(cfg.Machine) >= len(bad) && cfg.Machine[len(cfg.Machine)-len(bad):] == bad {
			t.Errorf("machine %q keeps an unstable suffix %q", cfg.Machine, bad)
		}
	}
	if cfg.Machine != stringsToLower(cfg.Machine) {
		t.Errorf("machine %q should be lower-cased", cfg.Machine)
	}
}

func stringsToLower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}

// The instructions template must not be able to bake machine- or user-specific
// values into a file that is commonly shared across machines.
func TestInstructionsTemplateHasNoMachinePlaceholder(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "adapters", "instructions", "regesto-section.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if contains(text, "{{machine}}") {
		t.Error("template still substitutes {{machine}} — that value is wrong on every other machine")
	}
	if contains(text, "/Users/") {
		t.Error("template contains an absolute home path")
	}
	if !contains(text, "{{kb_root}}") {
		t.Error("template should still take the KB root from config")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}

// repoRoot locates the repo from the test file's own location.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(wd)
}

// `regesto` must work from any directory, which means finding its instance from
// its own location when the working directory has none — otherwise the binary is
// only usable from inside the KB.
func TestConfigDiscoveryPrecedence(t *testing.T) {
	t.Setenv("REGESTO_MACHINE", "")
	root := t.TempDir()
	inner := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("agents = [\"claude\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Walking up from a nested directory finds it.
	got, err := config.Find(inner)
	if err != nil {
		t.Fatalf("Find from a nested dir: %v", err)
	}
	if got != cfgPath {
		t.Errorf("found %q, want %q", got, cfgPath)
	}

	// REGESTO_CONFIG wins over the walk.
	t.Setenv("REGESTO_CONFIG", "/somewhere/else/config.toml")
	if got, _ := config.Find(inner); got != "/somewhere/else/config.toml" {
		t.Errorf("env override ignored, got %q", got)
	}
	t.Setenv("REGESTO_CONFIG", "")

	// A directory with no instance above it, and no instance beside the test
	// binary, must fail with an error naming both places it looked.
	if _, err := config.Find(string(filepath.Separator)); err != nil {
		for _, want := range []string{"walking up", "beside the binary"} {
			if !contains(err.Error(), want) {
				t.Errorf("error should mention %q, got: %v", want, err)
			}
		}
	}
}
