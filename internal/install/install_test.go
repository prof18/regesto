package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/config"
)

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func loadTestConfig(t *testing.T, root, body string) *config.Config {
	t.Helper()
	for _, relative := range []string{"adapters/claude/hooks/session-start.sh", "adapters/hermes/hooks/pre-llm.sh"} {
		hook, err := regesto.Adapters.ReadFile(relative)
		if err != nil {
			t.Fatal(err)
		}
		hookPath := filepath.Join(root, filepath.FromSlash(relative))
		writeTestFile(t, hookPath, string(hook))
		if err := os.Chmod(hookPath, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "config.toml")
	writeTestFile(t, path, body)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func instructionItems(plan *Plan) []Item {
	var out []Item
	for _, item := range plan.Items {
		if item.Kind == "instructions" {
			out = append(out, item)
		}
	}
	return out
}

func TestCanonicalTargetResolvesMissingSuffixThroughSymlink(t *testing.T) {
	base := t.TempDir()
	realParent := filepath.Join(base, "outside")
	if err := os.MkdirAll(realParent, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "declared")
	if err := os.Symlink(realParent, link); err != nil {
		t.Fatal(err)
	}
	got, err := CanonicalTarget(filepath.Join(link, "missing", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	resolvedParent, err := filepath.EvalSymlinks(realParent)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(resolvedParent, "missing", "AGENTS.md")
	if got != want {
		t.Fatalf("canonical target = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(realParent, "missing")); !os.IsNotExist(err) {
		t.Fatalf("planning created the missing suffix: %v", err)
	}
}

func TestInstructionsGroupOwnersBackupOnceAndBecomeCurrent(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(home, "shared", "AGENTS.md")
	writeTestFile(t, target, "foreign instructions\n")
	cfg := loadTestConfig(t, root, "integrations = [\"zeta\", \"alpha\"]\n"+
		"[integrations.zeta]\nprofile = \"generic\"\ninstructions_file = \""+target+"\"\n"+
		"[integrations.alpha]\nprofile = \"generic\"\ninstructions_file = \""+target+"\"\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	items := instructionItems(plan)
	if len(items) != 1 {
		t.Fatalf("instruction items = %d, want one: %+v", len(items), items)
	}
	if got := strings.Join(items[0].Owners, ","); got != "alpha,zeta" {
		t.Fatalf("owners = %q, want alpha,zeta", got)
	}
	result, err := Apply(plan)
	if err != nil {
		t.Fatal(err)
	}
	targetDir, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Backups) != 1 || filepath.Dir(result.Backups[0]) != targetDir {
		t.Fatalf("backups = %v, want one beside target", result.Backups)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "foreign instructions\n") || strings.Count(string(body), sectionStart) != 1 {
		t.Fatalf("foreign content or marker damaged:\n%s", body)
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("second plan has %d changes", second.Changes())
	}
}

func TestInstructionsConflictFailsDeterministically(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(home, "shared", "AGENTS.md")
	writeTestFile(t, target, "foreign\n")
	cfg := loadTestConfig(t, root, "integrations = [\"zeta\", \"alpha\"]\n"+
		"[integrations.zeta]\nprofile = \"generic\"\ninstructions_file = \""+target+"\"\n"+
		"[integrations.alpha]\nprofile = \"generic\"\ninstructions_file = \""+target+"\"\n")
	_, err := Build(cfg, Options{InstructionSections: map[string][]byte{
		"alpha": []byte(sectionStart + "\nalpha\n" + sectionEnd + "\n"),
		"zeta":  []byte(sectionStart + "\nzeta\n" + sectionEnd + "\n"),
	}})
	if err == nil || !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "zeta") || !strings.Contains(err.Error(), target) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestSharedMissingInstructionsResolveSymlinkAncestorWithoutPlanningWrites(t *testing.T) {
	root, home, outside := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	link := filepath.Join(home, "dotfiles")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	declared := filepath.Join(link, "nested", "AGENTS.md")
	profile := `{"schema_version":1,"id":"maker","display_name":"Maker","skills":{"targets":[],"variant":"portable"},"instructions":{"targets":[],"create":true},"hooks":[{"protocol":"none","registrar":"none"}],"memory":[{"kind":"none"}],"default_trust":"quarantine"}`
	writeTestFile(t, filepath.Join(root, "adapters", "profiles", "maker.json"), profile)
	cfg := loadTestConfig(t, root, "integrations = [\"beta\", \"alpha\"]\n"+
		"[integrations.beta]\nprofile = \"maker\"\ninstructions_file = \""+declared+"\"\n"+
		"[integrations.alpha]\nprofile = \"maker\"\ninstructions_file = \""+declared+"\"\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	items := instructionItems(plan)
	outsideResolved, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(outsideResolved, "nested", "AGENTS.md")
	if len(items) != 1 || items[0].Action != "create" || items[0].CanonicalTarget != want || strings.Join(items[0].Owners, ",") != "alpha,beta" {
		t.Fatalf("missing shared plan = %+v", items)
	}
	if _, err := os.Stat(filepath.Join(outside, "nested")); !os.IsNotExist(err) {
		t.Fatalf("planning created missing target: %v", err)
	}
	result, err := Apply(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Backups) != 0 {
		t.Fatalf("new target should not be backed up: %v", result.Backups)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("missing target was not created: %v", err)
	}
}

func TestInstructionsOutsideHomeBackupAndSymlinkSwapRefusal(t *testing.T) {
	root, home, outside := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	other := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "AGENTS.md"), "outside A\n")
	writeTestFile(t, filepath.Join(other, "AGENTS.md"), "outside B\n")
	link := filepath.Join(home, "shared")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	declared := filepath.Join(link, "AGENTS.md")
	cfg := loadTestConfig(t, root, "integrations = [\"custom\"]\n[integrations.custom]\nprofile = \"generic\"\ninstructions_file = \""+declared+"\"\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	items := instructionItems(plan)
	outsideResolved, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].CanonicalTarget != filepath.Join(outsideResolved, "AGENTS.md") {
		t.Fatalf("outside canonical target not disclosed: %+v", items)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan); err == nil || !strings.Contains(err.Error(), "target changed after planning") {
		t.Fatalf("swap apply error = %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(outside, "AGENTS.md.regesto-backup.*")); len(matches) != 0 {
		t.Fatalf("swap refusal created backup: %v", matches)
	}

	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	plan, err = Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Apply(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Backups) != 1 || filepath.Dir(result.Backups[0]) != outsideResolved {
		t.Fatalf("backup = %v, want beside resolved outside target", result.Backups)
	}
}

func TestHookSettingsRejectDuplicateJSONKeysBeforePlanningWrites(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	settings := filepath.Join(home, ".claude", "settings.json")
	writeTestFile(t, settings, `{"hooks":{},"hooks":{"foreign":true}}`)
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	_, err := Build(cfg, Options{})
	if err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("duplicate settings error = %v", err)
	}
	if got := string(mustReadTest(t, settings)); got != `{"hooks":{},"hooks":{"foreign":true}}` {
		t.Fatalf("malformed settings changed during planning: %s", got)
	}
}

func TestHookSettingsAlreadyRegisteredRemainByteForByteUnchanged(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	settings := filepath.Join(home, ".claude", "settings.json")
	command := filepath.Join(root, "adapters", "claude", "hooks", "session-start.sh")
	body := "{\n  \"foreign\" : true,\n  \"hooks\" : {\"SessionStart\":[{\"hooks\":[{\"type\":\"command\",\"command\":\"" + command + "\"}]}]}\n}\n"
	writeTestFile(t, settings, body)
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.Items {
		if item.Kind == "hook" && item.Action != "current" {
			t.Fatalf("registered hook plan = %+v", item)
		}
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadTest(t, settings)); got != body {
		t.Fatalf("registered settings reformatted:\n%s", got)
	}
}

func TestInstallRefusesRenderedStageSymlinkOutsideKB(t *testing.T) {
	root, home, outside := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(root, ".state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".state", "skills")); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, root, "integrations = []\n")
	_, err := Build(cfg, Options{})
	if err == nil || !strings.Contains(err.Error(), "outside the knowledge base") {
		t.Fatalf("outside stage error = %v", err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("outside stage changed: entries=%v err=%v", entries, readErr)
	}
}

func TestInstallPreservesUnownedDirectoryInRenderedStage(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	personal := filepath.Join(root, ".state", "skills", "my-personal", "notes.txt")
	writeTestFile(t, personal, "keep me\n")
	cfg := loadTestConfig(t, root, "integrations = []\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadTest(t, personal)); got != "keep me\n" {
		t.Fatalf("unowned stage content changed: %q", got)
	}
}

func TestInstallRefusesToClaimShippedStageDirectoryWithUnknownFiles(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	foreign := filepath.Join(root, ".state", "integrations", "claude", "skills", "regesto-search", "notes.txt")
	writeTestFile(t, foreign, "foreign\n")
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	_, err := Build(cfg, Options{})
	if err == nil || !strings.Contains(err.Error(), "unowned rendered skill directory") {
		t.Fatalf("unowned shipped-stage error = %v", err)
	}
	if got := string(mustReadTest(t, foreign)); got != "foreign\n" {
		t.Fatalf("foreign shipped-stage file changed: %q", got)
	}
}

func TestInstallRefusesToClaimShippedStageDirectoryWithUnexpectedEmptyDirectory(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	foreign := filepath.Join(root, ".state", "integrations", "claude", "skills", "regesto-search", "empty-foreign")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	_, err := Build(cfg, Options{})
	if err == nil || !strings.Contains(err.Error(), "unowned rendered skill directory") {
		t.Fatalf("unowned shipped-stage empty-directory error = %v", err)
	}
	if info, err := os.Stat(foreign); err != nil || !info.IsDir() {
		t.Fatalf("foreign empty directory was not preserved: info=%v err=%v", info, err)
	}
}

func TestInstallRejectsCrossKindCanonicalTargetConflict(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	settings := filepath.Join(home, ".claude", "settings.json")
	writeTestFile(t, settings, "{}\n")
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n[integrations.claude]\ninstructions_file = \""+settings+"\"\n")
	_, err := Build(cfg, Options{})
	if err == nil || !strings.Contains(err.Error(), "install target conflict") || !strings.Contains(err.Error(), "hook:") || !strings.Contains(err.Error(), "instructions:") {
		t.Fatalf("cross-kind conflict error = %v", err)
	}
	if got := string(mustReadTest(t, settings)); got != "{}\n" {
		t.Fatalf("conflict planning changed target: %q", got)
	}
}

func TestLegacySettingsOverrideStillRegistersSessionHook(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	settings := filepath.Join(home, ".mystery", "settings.json")
	cfg := loadTestConfig(t, root, "agents = [\"mystery\"]\n[settings_files]\nmystery = \""+settings+"\"\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range plan.Items {
		if item.Kind == "hook" && strings.Join(item.Owners, ",") == "mystery" && item.CanonicalTarget == mustCanonical(t, settings) {
			found = true
		}
	}
	if !found {
		t.Fatalf("legacy settings override has no hook plan: %+v", plan.Items)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mustReadTest(t, settings)), "SessionStart") {
		t.Fatal("legacy settings override did not receive SessionStart hook")
	}
}

func TestInstallRejectsMissingOrNonExecutableHook(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	hook := filepath.Join(root, "adapters", "claude", "hooks", "session-start.sh")
	if err := os.Chmod(hook, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(cfg, Options{}); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("non-executable hook error = %v", err)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(cfg, Options{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing hook error = %v", err)
	}
}

func TestRetiredSkillDirectoryAndLiveLinkPruneInOneInstall(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	canonicalRoot := mustCanonical(t, root)
	retiredStage := filepath.Join(root, ".state", "skills", "retired")
	writeTestFile(t, filepath.Join(retiredStage, ".regesto-owned"), string(stageMarker(canonicalRoot, "retired")))
	writeTestFile(t, filepath.Join(retiredStage, "SKILL.md"), "retired\n")
	link := filepath.Join(home, ".claude", "skills", "retired")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(retiredStage, link); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{retiredStage, link} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("retired path remains at %s: %v", path, err)
		}
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("second plan has %d changes", second.Changes())
	}
}

func TestLegacyRenderedSkillMigratesToIntegrationTree(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	canonicalRoot := mustCanonical(t, root)
	legacy := filepath.Join(root, ".state", "skills", "regesto-search")
	writeTestFile(t, filepath.Join(legacy, ".regesto-owned"), string(stageMarker(canonicalRoot, "regesto-search")))
	writeTestFile(t, filepath.Join(legacy, "SKILL.md"), "legacy rendered skill\n")
	link := filepath.Join(home, ".claude", "skills", "regesto-search")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(legacy, link); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy stage remains: %v", err)
	}
	want := filepath.Join(root, ".state", "integrations", "claude", "skills", "regesto-search")
	if got, err := os.Readlink(link); err != nil || got != want {
		t.Fatalf("migrated skill link = %q, err=%v, want %q", got, err, want)
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("second migrated plan has %d changes", second.Changes())
	}
}

func TestRetiredSkillDirectoryChangeAfterPlanIsRefused(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	canonicalRoot := mustCanonical(t, root)
	retired := filepath.Join(root, ".state", "skills", "retired")
	writeTestFile(t, filepath.Join(retired, ".regesto-owned"), string(stageMarker(canonicalRoot, "retired")))
	writeTestFile(t, filepath.Join(retired, "SKILL.md"), "retired\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(retired, "new-after-plan.txt"), "new\n")
	if _, err := Apply(plan); err == nil || !strings.Contains(err.Error(), "contents changed after planning") {
		t.Fatalf("changed retired-stage apply error = %v", err)
	}
	if got := string(mustReadTest(t, filepath.Join(retired, "new-after-plan.txt"))); got != "new\n" {
		t.Fatalf("new retired-stage file was not preserved: %q", got)
	}
}

func TestStaleFileInsideShippedSkillPrunesInOneInstall(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	initial, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(initial); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, ".state", "integrations", "claude", "skills", "regesto-search", "old.md")
	writeTestFile(t, stale, "old\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale rendered file remains: %v", err)
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("second plan has %d changes", second.Changes())
	}
}

func TestStaleGeneratedFileChangeAfterPlanIsRefused(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	initial, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(initial); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, ".state", "integrations", "claude", "skills", "regesto-search", "old.md")
	writeTestFile(t, stale, "old\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, stale, "changed after plan\n")
	if _, err := Apply(plan); err == nil || !strings.Contains(err.Error(), "changed after planning") {
		t.Fatalf("changed stale-file apply error = %v", err)
	}
	if got := string(mustReadTest(t, stale)); got != "changed after plan\n" {
		t.Fatalf("changed stale file was not preserved: %q", got)
	}
}

func TestRenderedSkillOwnershipChangeAfterPlanIsRefused(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	initial, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(initial); err != nil {
		t.Fatal(err)
	}
	rendered := filepath.Join(root, ".state", "integrations", "claude", "skills", "regesto-search", "SKILL.md")
	if err := os.Remove(rendered); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".state", "integrations", "claude", "skills", "regesto-search", ".regesto-owned")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan); err == nil || !strings.Contains(err.Error(), "ownership marker changed after planning") {
		t.Fatalf("changed ownership-marker apply error = %v", err)
	}
	if _, err := os.Stat(rendered); !os.IsNotExist(err) {
		t.Fatalf("rendered file was created after ownership changed: %v", err)
	}
}

func TestHermesMissingConfigAndAllowlistInstallIdempotently(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"hermes\"]\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(root, "adapters", "hermes", "hooks", "pre-llm.sh")
	configBody := string(mustReadTest(t, filepath.Join(home, ".hermes", "config.yaml")))
	if !strings.Contains(configBody, "pre_llm_call") || !strings.Contains(configBody, command) {
		t.Fatalf("Hermes config missing exact registration:\n%s", configBody)
	}
	allowlistBody := string(mustReadTest(t, filepath.Join(home, ".hermes", "shell-hooks-allowlist.json")))
	if !strings.Contains(allowlistBody, `"event": "pre_llm_call"`) || !strings.Contains(allowlistBody, command) {
		t.Fatalf("Hermes allowlist missing exact approval:\n%s", allowlistBody)
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("second Hermes plan has %d changes", second.Changes())
	}
}

func TestHermesExistingYAMLIsPreservedWithExactManualRecipe(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".hermes", "config.yaml")
	writeTestFile(t, configPath, "model: custom\nforeign: true\n")
	allowlistPath := filepath.Join(home, ".hermes", "shell-hooks-allowlist.json")
	writeTestFile(t, allowlistPath, "{\n  \"approvals\": [{\"event\": \"post_llm_call\", \"command\": \"/foreign\"}],\n  \"foreign\": true\n}\n")
	cfg := loadTestConfig(t, root, "integrations = [\"hermes\"]\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	manual := false
	for _, item := range plan.Items {
		if item.ID == "hook-hermes-config:"+mustCanonical(t, configPath) {
			manual = item.Action == "manual" && strings.Contains(item.IntendedState, "pre_llm_call") && strings.Contains(item.IntendedState, "pre-llm.sh")
		}
	}
	if !manual {
		t.Fatalf("Hermes manual recipe missing: %+v", plan.Items)
	}
	result, err := Apply(plan)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadTest(t, configPath)); got != "model: custom\nforeign: true\n" {
		t.Fatalf("Hermes YAML changed:\n%s", got)
	}
	if len(result.Backups) != 1 || filepath.Dir(result.Backups[0]) != filepath.Dir(mustCanonical(t, allowlistPath)) {
		t.Fatalf("Hermes allowlist backups = %v", result.Backups)
	}
	allowlist := string(mustReadTest(t, allowlistPath))
	if !strings.Contains(allowlist, "/foreign") || !strings.Contains(allowlist, "pre_llm_call") || !strings.Contains(allowlist, `"foreign": true`) {
		t.Fatalf("Hermes allowlist foreign content lost:\n%s", allowlist)
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("second existing-YAML Hermes plan has %d changes", second.Changes())
	}
}

func TestHermesSharedTargetsGroupStableOwners(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"z-hermes\", \"a-hermes\"]\n"+
		"[integrations.z-hermes]\nprofile = \"hermes\"\n"+
		"[integrations.a-hermes]\nprofile = \"hermes\"\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, item := range plan.Items {
		if strings.HasPrefix(item.ID, "hook-hermes-") {
			seen++
			if got := strings.Join(item.Owners, ","); got != "a-hermes,z-hermes" {
				t.Fatalf("Hermes shared owners = %q", got)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("Hermes shared hook items = %d, want config and allowlist", seen)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
}

func TestHermesAllowlistRejectsDuplicateKeysWithoutWrites(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	allowlist := filepath.Join(home, ".hermes", "shell-hooks-allowlist.json")
	writeTestFile(t, allowlist, `{"approvals":[],"approvals":[]}`)
	cfg := loadTestConfig(t, root, "integrations = [\"hermes\"]\n")
	if _, err := Build(cfg, Options{}); err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("duplicate Hermes allowlist error = %v", err)
	}
	if got := string(mustReadTest(t, allowlist)); got != `{"approvals":[],"approvals":[]}` {
		t.Fatalf("duplicate Hermes allowlist changed: %q", got)
	}
}

func TestHermesCommandIsShellQuotedForShlexSplit(t *testing.T) {
	if got := shellQuote("/tmp/plain/path"); got != "'/tmp/plain/path'" {
		t.Fatalf("plain shell quote = %q", got)
	}
	if got := shellQuote("/tmp/root with 'quote'/hook"); got != `'/tmp/root with '"'"'quote'"'"'/hook'` {
		t.Fatalf("complex shell quote = %q", got)
	}
	if got := shellQuote("/tmp/$HOME;touch owned|hook&other*(?)"); got != "'/tmp/$HOME;touch owned|hook&other*(?)'" {
		t.Fatalf("metacharacter shell quote = %q", got)
	}
}

func TestOwnershipMarkerIsAppliedBeforeEveryRenderedPayload(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, filepath.Join(root, "adapters", "skills", "order-test", "SKILL.md"), "---\nname: order-test\ndescription: Verify generated ownership ordering.\n---\n\npayload\n")
	cfg := loadTestConfig(t, root, "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nskills_dir = \""+filepath.Join(home, "maker", "skills")+"\"\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	markerIndex, payloadIndex := -1, -1
	for i, item := range plan.Items {
		switch item.ID {
		case "skill-render:maker:order-test:.regesto-owned":
			markerIndex = i
		case "skill-render:maker:order-test:SKILL.md":
			payloadIndex = i
		}
	}
	if markerIndex < 0 || payloadIndex < 0 || markerIndex >= payloadIndex {
		t.Fatalf("ownership marker order = %d, payload order = %d", markerIndex, payloadIndex)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadTest(t, filepath.Join(root, ".state", "integrations", "maker", "skills", "order-test", "SKILL.md"))); !strings.Contains(got, "payload") {
		t.Fatalf("rendered payload = %q", got)
	}
}

func TestInstanceSkillWithNonPortableFrontmatterRequiresUpgrade(t *testing.T) {
	for _, test := range []struct {
		name  string
		field string
	}{
		{name: "when-to-use", field: "when_to_use: legacy trigger"},
		{name: "allowed-tools", field: "allowed-tools: Bash"},
		{name: "malformed-scalar", field: "compatibility: [unterminated"},
		{name: "malformed-metadata", field: "metadata:\n  missing-colon"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, home := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			body := "---\nname: legacy-skill\ndescription: Legacy shipped skill.\n" + test.field + "\n---\n\nLegacy.\n"
			writeTestFile(t, filepath.Join(root, "adapters", "skills", "legacy-skill", "SKILL.md"), body)
			cfg := loadTestConfig(t, root, "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nskills_dir = \""+filepath.Join(home, "maker", "skills")+"\"\n")
			_, err := Build(cfg, Options{})
			if err == nil || !strings.Contains(err.Error(), "regesto upgrade") {
				t.Fatalf("legacy skill error = %v", err)
			}
		})
	}
}

func TestPortableSkillValidationRejectsMalformedYAMLScalars(t *testing.T) {
	for name, body := range map[string]string{
		"unterminated collection": "---\nname: sample\ndescription: [unterminated\n---\n",
		"unterminated quote":      "---\nname: sample\ndescription: \"unterminated\n---\n",
		"invalid metadata":        "---\nname: sample\ndescription: Valid.\nmetadata:\n  missing-colon\n---\n",
		"comment value":           "---\nname: sample\ndescription: # not a string\n---\n",
		"sequence value":          "---\nname: sample\ndescription: - not a scalar\n---\n",
		"boolean metadata":        "---\nname: sample\ndescription: Valid.\nmetadata:\n  enabled: true\n---\n",
		"numeric metadata":        "---\nname: sample\ndescription: Valid.\nmetadata:\n  count: 123\n---\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePortableSkill("sample", []byte(body)); err == nil {
				t.Fatal("malformed portable frontmatter was accepted")
			}
		})
	}
}

func TestPortableSkillValidationAcceptsBlockDescription(t *testing.T) {
	body := "---\nname: sample\ndescription: >\n  Search prior decisions and conventions before\n  answering questions that recorded knowledge can settle.\ncompatibility: 'Requires a POSIX-compatible shell.'\n---\n"
	if err := validatePortableSkill("sample", []byte(body)); err != nil {
		t.Fatalf("valid block-scalar frontmatter rejected: %v", err)
	}
}

func TestPortableSkillValidationEnforcesCompatibilityLength(t *testing.T) {
	for name, compatibility := range map[string]string{
		"empty":    `""`,
		"too-long": `"` + strings.Repeat("x", 501) + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			body := "---\nname: sample\ndescription: Valid.\ncompatibility: " + compatibility + "\n---\n"
			if err := validatePortableSkill("sample", []byte(body)); err == nil {
				t.Fatal("invalid compatibility was accepted")
			}
		})
	}
}

func TestDuplicateLegacyIntegrationRendersAndAppliesOnce(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "agents = [\"claude\", \"claude\"]\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("duplicate legacy second plan has %d changes", second.Changes())
	}
}

func TestInstallRefusesSymlinkedRenderedSkillDirectory(t *testing.T) {
	root, home, foreign := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, filepath.Join(foreign, "SKILL.md"), "foreign\n")
	dir := filepath.Join(root, ".state", "integrations", "claude", "skills", "regesto-search")
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, dir); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	_, err := Build(cfg, Options{})
	if err == nil || !strings.Contains(err.Error(), "symlinked rendered skill directory") {
		t.Fatalf("symlinked rendered directory error = %v", err)
	}
	if got := string(mustReadTest(t, filepath.Join(foreign, "SKILL.md"))); got != "foreign\n" {
		t.Fatalf("foreign symlink target changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(foreign, ".regesto-owned")); !os.IsNotExist(err) {
		t.Fatalf("foreign symlink target was claimed: %v", err)
	}
}

func TestInstallRefusesSymlinkedRenderedSkillPayload(t *testing.T) {
	root, home, foreign := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, filepath.Join(root, "adapters", "skills", "sample", "SKILL.md"), "---\nname: sample\ndescription: Sample.\n---\n")
	stage := filepath.Join(root, ".state", "integrations", "maker", "skills", "sample")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := CanonicalTarget(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, ".regesto-owned"), stageMarker(canonicalRoot, "maker", "sample"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreignFile := filepath.Join(foreign, "payload.md")
	if err := os.WriteFile(foreignFile, []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreignFile, filepath.Join(stage, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, root, "integrations = [\"maker\"]\n[integrations.maker]\nprofile = \"generic\"\nskills_dir = \""+filepath.Join(home, "maker", "skills")+"\"\n")
	if _, err := Build(cfg, Options{}); err == nil || !strings.Contains(err.Error(), "symlinked rendered skill payload") {
		t.Fatalf("symlinked payload error = %v", err)
	}
	if got := string(mustReadTest(t, foreignFile)); got != "foreign" {
		t.Fatalf("foreign payload changed to %q", got)
	}
}

func TestAnchoredRootCannotBeRedirectedAfterOpen(t *testing.T) {
	base, foreign := t.TempDir(), t.TempDir()
	canonicalBase, err := CanonicalTarget(base)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(canonicalBase, "target")
	moved := filepath.Join(canonicalBase, "moved")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := openAnchoredRoot(target, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(target, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, target); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("payload", []byte("anchored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadTest(t, filepath.Join(moved, "payload"))); got != "anchored" {
		t.Fatalf("anchored payload = %q", got)
	}
	if _, err := os.Stat(filepath.Join(foreign, "payload")); !os.IsNotExist(err) {
		t.Fatalf("redirected root received payload: %v", err)
	}
}

func TestCrossSkillStageLinkIsPreservedWithoutMatchingOwnershipProof(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"claude\"]\n")
	initial, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(initial); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".claude", "skills", "regesto-search")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	foreignTarget := filepath.Join(root, ".state", "integrations", "claude", "skills", "regesto-write")
	if err := os.Symlink(foreignTarget, link); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range plan.Items {
		if item.Kind == "skill-link" && item.CanonicalTarget == mustCanonicalLink(t, link) {
			found = true
			if item.Action != "skip" {
				t.Fatalf("cross-skill stage link action = %s, want skip", item.Action)
			}
		}
	}
	if !found {
		t.Fatal("cross-skill stage link missing from plan")
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != foreignTarget {
		t.Fatalf("cross-skill stage link = %q, err=%v", got, err)
	}
}

func TestEnginePathLinkIsPlannedAppliedAndIdempotent(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = []\n")
	target := filepath.Join(root, "bin", "regesto")
	writeTestFile(t, target, "engine\n")
	link := filepath.Join(home, ".local", "bin", "regesto")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := Options{EngineLink: link, EngineTarget: target}
	plan, err := Build(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range plan.Items {
		if item.Kind == "engine-link" && item.Action == "create" {
			found = true
		}
	}
	if !found {
		t.Fatalf("engine link missing from plan: %+v", plan.Items)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("engine link = %q, err=%v", got, err)
	}
	second, err := Build(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changes() != 0 {
		t.Fatalf("second plan has %d changes", second.Changes())
	}
}

func mustCanonical(t *testing.T, path string) string {
	t.Helper()
	canonical, err := CanonicalTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func mustCanonicalLink(t *testing.T, path string) string {
	t.Helper()
	canonical, err := canonicalLinkPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func mustReadTest(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
