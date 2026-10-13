package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func piHookItem(t *testing.T, plan *Plan) *Item {
	t.Helper()
	for i := range plan.Items {
		if plan.Items[i].Kind == "hook-link" && strings.HasSuffix(plan.Items[i].CanonicalTarget, filepath.FromSlash(".pi/agent/extensions/regesto.ts")) {
			return &plan.Items[i]
		}
	}
	t.Fatalf("no pi hook-link item in plan:\n%v", plan.Items)
	return nil
}

func TestPiExtensionLinkInstallsAndIsIdempotent(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"pi\"]\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	item := piHookItem(t, plan)
	want := filepath.Join(root, "adapters", "pi", "extensions", "regesto.ts")
	if item.Action != "create" || item.linkTarget != want || item.CanonicalTarget != mustCanonicalLink(t, filepath.Join(home, ".pi/agent/extensions/regesto.ts")) {
		t.Fatalf("unexpected plan item: %+v", item)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".pi", "agent", "extensions", "regesto.ts")
	raw, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("extension link missing after apply: %v", err)
	}
	if filepath.Clean(raw) != want {
		t.Fatalf("extension link points at %s, want %s", raw, want)
	}
	body, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(body), "pi-before-agent-start-v1") {
		t.Fatalf("linked extension is not the launcher: %v\n%s", err, body)
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := piHookItem(t, second); got.Action != "current" {
		t.Fatalf("second plan action = %q, want current", got.Action)
	}
	if second.Changes() != 0 {
		t.Fatalf("second pi plan has %d changes", second.Changes())
	}
}

func TestPiExtensionLinkLeavesForeignTargetsAlone(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"pi\"]\n")
	target := filepath.Join(home, ".pi", "agent", "extensions", "regesto.ts")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "mine.ts"), target); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	item := piHookItem(t, plan)
	if item.Action != "skip" {
		t.Fatalf("action = %q, want skip", item.Action)
	}
	before := targetState(t, target)
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if after := targetState(t, target); after != before {
		t.Fatalf("foreign target disturbed:\nbefore %s\nafter %s", before, after)
	}
}

func TestPiExtensionLinkRepairsStaleInstanceLink(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"pi\"]\n")
	target := filepath.Join(home, ".pi", "agent", "extensions", "regesto.ts")
	// A link left behind by an instance that has since moved or been removed:
	// regesto-shaped, but its knowledge base is no longer there.
	gone := filepath.Join(t.TempDir(), "gone-kb")
	stale := filepath.Join(gone, "adapters", "pi", "extensions", "regesto.ts")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stale, target); err != nil {
		t.Fatal(err)
	}
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	item := piHookItem(t, plan)
	if item.Action != "replace" || !strings.Contains(item.CurrentState, "no longer there") {
		t.Fatalf("stale link plan = %q / %q, want replace", item.Action, item.CurrentState)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "adapters", "pi", "extensions", "regesto.ts")
	raw, err := os.Readlink(target)
	if err != nil || filepath.Clean(raw) != want {
		t.Fatalf("stale link repaired to %q (%v), want %q", raw, err, want)
	}
}

func TestPiProtocolCommandNeedsNoExecutableBit(t *testing.T) {
	root := t.TempDir()
	extension := filepath.Join(root, "adapters", "pi", "extensions", "regesto.ts")
	writeTestFile(t, extension, "// launcher\n")
	command, event, err := protocolHookCommand(root, root, "pi-before-agent-start-v1")
	if err != nil {
		t.Fatal(err)
	}
	if command != extension || event != "before_agent_start" {
		t.Fatalf("command=%q event=%q", command, event)
	}
}

func TestOtherAdapterKnowledgeBaseRecognizesEveryLauncher(t *testing.T) {
	for _, relative := range launcherRelatives {
		kb := filepath.Join(t.TempDir(), "kb")
		target := filepath.Join(kb, filepath.FromSlash(relative))
		if got := OtherAdapterKnowledgeBase(target); got != kb {
			t.Fatalf("OtherAdapterKnowledgeBase(%s) = %q, want %q", target, got, kb)
		}
	}
	if got := OtherAdapterKnowledgeBase(filepath.Join(t.TempDir(), "elsewhere", "regesto.ts")); got != "" {
		t.Fatalf("foreign target reported knowledge base %q", got)
	}
}
