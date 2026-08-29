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
	hook, err := regesto.Adapters.ReadFile("adapters/claude/hooks/session-start.sh")
	if err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(root, "adapters", "claude", "hooks", "session-start.sh")
	writeTestFile(t, hookPath, string(hook))
	if err := os.Chmod(hookPath, 0o755); err != nil {
		t.Fatal(err)
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
	foreign := filepath.Join(root, ".state", "skills", "regesto-search", "notes.txt")
	writeTestFile(t, foreign, "foreign\n")
	cfg := loadTestConfig(t, root, "integrations = []\n")
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
	foreign := filepath.Join(root, ".state", "skills", "regesto-search", "empty-foreign")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := loadTestConfig(t, root, "integrations = []\n")
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

func TestRetiredSkillDirectoryChangeAfterPlanIsRefused(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = []\n")
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
	cfg := loadTestConfig(t, root, "integrations = []\n")
	initial, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(initial); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, ".state", "skills", "regesto-search", "old.md")
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
	cfg := loadTestConfig(t, root, "integrations = []\n")
	initial, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(initial); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, ".state", "skills", "regesto-search", "old.md")
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
	cfg := loadTestConfig(t, root, "integrations = []\n")
	initial, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(initial); err != nil {
		t.Fatal(err)
	}
	rendered := filepath.Join(root, ".state", "skills", "regesto-search", "SKILL.md")
	if err := os.Remove(rendered); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".state", "skills", "regesto-search", ".regesto-owned")
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

func TestOwnershipMarkerIsAppliedBeforeEveryRenderedPayload(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, filepath.Join(root, "adapters", "skills", "order-test", "!payload"), "payload\n")
	cfg := loadTestConfig(t, root, "integrations = []\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	markerIndex, payloadIndex := -1, -1
	for i, item := range plan.Items {
		switch item.ID {
		case "skill-render:order-test:.regesto-owned":
			markerIndex = i
		case "skill-render:order-test:!payload":
			payloadIndex = i
		}
	}
	if markerIndex < 0 || payloadIndex < 0 || markerIndex >= payloadIndex {
		t.Fatalf("ownership marker order = %d, payload order = %d", markerIndex, payloadIndex)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadTest(t, filepath.Join(root, ".state", "skills", "order-test", "!payload"))); got != "payload\n" {
		t.Fatalf("rendered payload = %q", got)
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
	foreignTarget := filepath.Join(root, ".state", "skills", "regesto-write")
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
