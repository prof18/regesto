package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/config"
	regestoinstall "github.com/prof18/regesto/internal/install"
)

type installJSON struct {
	SchemaVersion int                 `json:"schema_version"`
	DryRun        bool                `json:"dry_run"`
	Plan          regestoinstall.Plan `json:"plan"`
}

func installConfig(t *testing.T, root string, agents string) *config.Config {
	t.Helper()
	materializeInstallHook(t, root)
	path := filepath.Join(root, "config.toml")
	body := "machine = \"testbox\"\nagents = [" + agents + "]\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func materializeInstallHook(t *testing.T, root string) {
	t.Helper()
	files, err := regesto.InstanceFiles()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "adapters", "claude", "hooks", "session-start.sh")
	writeAt(t, root, "adapters/claude/hooks/session-start.sh", string(files["adapters/claude/hooks/session-start.sh"]))
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func installHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, dir := range []string{".claude", ".codex"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// treeSnapshot records regular files, directories, and symlink targets. It is
// intentionally small and local: the dry-run contract is that HOME is byte and
// directory-shape identical after the command.
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out[rel] = "link:" + target
			return nil
		}
		if info.IsDir() {
			out[rel] = "dir"
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = "file:" + string(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func agentSnapshot(t *testing.T, home string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, dir := range []string{".claude", ".codex"} {
		for path, value := range treeSnapshot(t, filepath.Join(home, dir)) {
			out[filepath.Join(dir, path)] = value
		}
	}
	return out
}

func runInstallJSON(t *testing.T, cfgPath string, args ...string) installJSON {
	t.Helper()
	cmdArgs := append([]string{"run", "./cmd/regesto", "--config", cfgPath, "install", "--json"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"), "GOTELEMETRY=off", "GOCACHE="+t.TempDir(), "GOPATH="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("install command: %v\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}
	var got installJSON
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid install JSON: %v\n%s", err, stdout.String())
	}
	return got
}

func TestInstallDryRunIsInertAndJSONIsComplete(t *testing.T) {
	home := installHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := installConfig(t, root, `"claude", "codex"`)
	before := agentSnapshot(t, home)
	beforeRoot := treeSnapshot(t, root)
	got := runInstallJSON(t, cfg.Path, "--dry-run")
	if got.SchemaVersion != 1 || !got.DryRun || got.Plan.KBRoot != root {
		t.Fatalf("unexpected dry-run envelope: %+v", got)
	}
	if got.Plan.Changes() == 0 {
		t.Fatal("dry-run unexpectedly planned no changes")
	}
	after := agentSnapshot(t, home)
	if len(after) != len(before) {
		t.Fatalf("dry-run changed HOME: before=%v after=%v", before, after)
	}
	for path, want := range before {
		if got := after[path]; got != want {
			t.Fatalf("dry-run changed %s: %q -> %q", path, want, got)
		}
	}
	afterRoot := treeSnapshot(t, root)
	if len(afterRoot) != len(beforeRoot) {
		t.Fatalf("dry-run changed instance: before=%v after=%v", beforeRoot, afterRoot)
	}
	for path, want := range beforeRoot {
		if got := afterRoot[path]; got != want {
			t.Fatalf("dry-run changed instance %s: %q -> %q", path, want, got)
		}
	}
	for _, item := range got.Plan.Items {
		if item.CanonicalTarget == "" || item.DryRun == "" || item.IntendedState == "" {
			t.Fatalf("plan item lacks disclosure: %+v", item)
		}
	}
}

func TestInstallApplyAndSecondPlanAreNoOp(t *testing.T) {
	home := installHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("foreign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := installConfig(t, root, `"claude"`)
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changes() == 0 {
		t.Fatal("initial plan unexpectedly has no changes")
	}
	if _, err := regestoinstall.Apply(plan); err != nil {
		t.Fatal(err)
	}
	second, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		var changes []string
		for _, item := range second.Items {
			if item.Action == "create" || item.Action == "update" || item.Action == "replace" || item.Action == "remove" {
				changes = append(changes, item.ID+":"+item.Action)
			}
		}
		t.Fatalf("second plan still mutates: %s", strings.Join(changes, ", "))
	}
}

func TestInstallSharedInstructionTargetUsesOneOwnerAndBackup(t *testing.T) {
	home := installHome(t)
	root := t.TempDir()
	materializeInstallHook(t, root)
	instructions := filepath.Join(home, ".dotfiles", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(instructions), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instructions, []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("machine = \"testbox\"\nagents = [\"claude\", \"codex\"]\n[instructions]\nclaude = \"~/.dotfiles/AGENTS.md\"\ncodex = \"~/.dotfiles/AGENTS.md\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var instruction *regestoinstall.Item
	for i := range plan.Items {
		if plan.Items[i].Kind == "instructions" {
			instruction = &plan.Items[i]
			break
		}
	}
	if instruction == nil || instruction.Action != "update" {
		t.Fatalf("shared instruction plan = %+v", instruction)
	}
	if strings.Join(instruction.Owners, ",") != "claude,codex" {
		t.Fatalf("owners = %v, want stable shared owners", instruction.Owners)
	}
	result, err := regestoinstall.Apply(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Backups) != 1 {
		t.Fatalf("backups = %v, want one shared-target backup", result.Backups)
	}
	backupBody, err := os.ReadFile(result.Backups[0])
	if err != nil {
		t.Fatal(err)
	}
	canonicalInstructions, err := regestoinstall.CanonicalTarget(instructions)
	if err != nil {
		t.Fatal(err)
	}
	if string(backupBody) != "keep me\n" || filepath.Dir(result.Backups[0]) != filepath.Dir(canonicalInstructions) {
		t.Fatalf("backup %q not beside target or did not preserve original", result.Backups[0])
	}
	if strings.Count(string(mustRead(t, instructions)), "regesto:section:start") != 1 {
		t.Fatal("shared instructions did not receive exactly one section")
	}
}

func TestInstallPreservesForeignSkillCollision(t *testing.T) {
	home := installHome(t)
	root := t.TempDir()
	collision := filepath.Join(home, ".claude", "skills", "regesto-search")
	if err := os.MkdirAll(filepath.Dir(collision), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(collision, []byte("foreign skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := installConfig(t, root, `"claude"`)
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.Items {
		if item.Kind == "skill-link" && strings.HasSuffix(item.CanonicalTarget, "regesto-search") && item.Action != "skip" {
			t.Fatalf("foreign collision plan = %+v", item)
		}
	}
	if _, err := regestoinstall.Apply(plan); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, collision)); got != "foreign skill\n" {
		t.Fatalf("foreign skill collision changed to %q", got)
	}
}

func TestInstallPreservesDanglingLinksWithoutOwnershipProof(t *testing.T) {
	home := installHome(t)
	root := t.TempDir()
	skills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(skills, "retired-regesto-skill")
	foreign := filepath.Join(skills, "foreign-skill")
	if err := os.Symlink(filepath.Join(root, ".state", "skills", "retired-regesto-skill"), owned); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), foreign); err != nil {
		t.Fatal(err)
	}
	cfg := installConfig(t, root, `"claude"`)
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := regestoinstall.Apply(plan); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(owned); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("unproven stage link was not preserved: info=%v err=%v", info, err)
	}
	if info, err := os.Lstat(foreign); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("foreign dangling link was not preserved: info=%v err=%v", info, err)
	}
}

func TestInstallCustomIntegrationUsesOnlyConfiguredPaths(t *testing.T) {
	home := installHome(t)
	root := t.TempDir()
	skills := filepath.Join(home, ".synthetic", "skills")
	instructions := filepath.Join(home, ".synthetic", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(instructions), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instructions, []byte("synthetic foreign content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "integrations = [\"synthetic\"]\n[integrations.synthetic]\nprofile = \"generic\"\nskills_dir = \"" + skills + "\"\ninstructions_file = \"" + instructions + "\"\n"
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := regestoinstall.Apply(plan); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"regesto-search", "regesto-write", "regesto-promote"} {
		if info, err := os.Lstat(filepath.Join(skills, name)); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("configured skill %s missing: info=%v err=%v", name, info, err)
		}
	}
	if !strings.Contains(string(mustRead(t, instructions)), "regesto:section:start") {
		t.Fatal("configured instruction target did not receive section")
	}
}

func TestInstallShimForwardsDryRunAndCustomConfigToCheckoutSource(t *testing.T) {
	files, err := regesto.InstanceFiles()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeAt(t, root, "bin/regesto-install", string(files["bin/regesto-install"]))
	shim := filepath.Join(root, "bin", "regesto-install")
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAt(t, root, "go.mod", "module fixture\n")
	tools := t.TempDir()
	writeAt(t, tools, "go", "#!/bin/sh\nprintf 'source:%s\\n' \"$*\"\n")
	if err := os.Chmod(filepath.Join(tools, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	pathBin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(pathBin, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(root, "custom.toml")
	cmd := exec.Command(shim, "--dry-run", "--config", custom, "--json")
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+tools+string(os.PathListSeparator)+pathBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install shim: %v\n%s", err, out)
	}
	want := "source:-C " + root + " run ./cmd/regesto --config " + custom + " install --dry-run --json --engine-link " + filepath.Join(pathBin, "regesto") + " --engine-target " + filepath.Join(root, "bin", "regesto")
	if !strings.Contains(string(out), want) {
		t.Fatalf("shim output = %q, want %q", out, want)
	}
}

func TestInstallShimFallsBackToExistingCheckoutEngineWithoutGo(t *testing.T) {
	files, err := regesto.InstanceFiles()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeAt(t, root, "bin/regesto-install", string(files["bin/regesto-install"]))
	shim := filepath.Join(root, "bin", "regesto-install")
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAt(t, root, "go.mod", "module fixture\n")
	writeAt(t, root, "bin/regesto", "#!/bin/sh\nprintf 'engine:%s\\n' \"$*\"\n")
	if err := os.Chmod(filepath.Join(root, "bin", "regesto"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	cmd := exec.Command(shim, "--dry-run")
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH=/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install shim fallback: %v\n%s", err, out)
	}
	want := "engine:--config " + filepath.Join(root, "config.toml") + " install --dry-run"
	if !strings.Contains(string(out), want) {
		t.Fatalf("fallback output = %q, want %q", out, want)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// Keep the helper's output deterministic if a future test reports snapshots.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
