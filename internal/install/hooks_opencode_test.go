package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func opencodeHookItem(t *testing.T, plan *Plan) *Item {
	t.Helper()
	for i := range plan.Items {
		if plan.Items[i].Kind == "hook-link" {
			return &plan.Items[i]
		}
	}
	t.Fatalf("no hook-link item in plan:\n%v", plan.Items)
	return nil
}

func TestOpencodePluginLinkInstallsAndIsIdempotent(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"opencode\"]\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	item := opencodeHookItem(t, plan)
	want := filepath.Join(root, "adapters", "opencode", "plugins", "regesto.ts")
	if item.Action != "create" || item.linkTarget != want || item.CanonicalTarget != mustCanonicalLink(t, filepath.Join(home, ".config/opencode/plugins/regesto.ts")) {
		t.Fatalf("unexpected plan item: %+v", item)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".config", "opencode", "plugins", "regesto.ts")
	raw, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("plugin link missing after apply: %v", err)
	}
	if filepath.Clean(raw) != want {
		t.Fatalf("plugin link points at %s, want %s", raw, want)
	}
	body, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(body), "opencode-chat-message-v1") {
		t.Fatalf("linked plugin is not the launcher: %v\n%s", err, body)
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := opencodeHookItem(t, second); got.Action != "current" {
		t.Fatalf("second plan action = %q, want current", got.Action)
	}
	if second.Changes() != 0 {
		t.Fatalf("second opencode plan has %d changes", second.Changes())
	}
}

func TestOpencodePluginLinkLeavesForeignTargetsAlone(t *testing.T) {
	for name, seed := range map[string]func(t *testing.T, target string){
		"foreign file": func(t *testing.T, target string) {
			writeTestFile(t, target, "module.exports = {}\n")
		},
		"foreign link": func(t *testing.T, target string) {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "mine.ts"), target); err != nil {
				t.Fatal(err)
			}
		},
		"other knowledge base": func(t *testing.T, target string) {
			other := t.TempDir()
			writeTestFile(t, filepath.Join(other, "config.toml"), "machine = \"otherbox\"\n")
			writeTestFile(t, filepath.Join(other, "adapters", "opencode", "plugins", "regesto.ts"), "// other\n")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(other, "adapters", "opencode", "plugins", "regesto.ts"), target); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			root, home := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			cfg := loadTestConfig(t, root, "integrations = [\"opencode\"]\n")
			target := filepath.Join(home, ".config", "opencode", "plugins", "regesto.ts")
			seed(t, target)
			plan, err := Build(cfg, Options{})
			if err != nil {
				t.Fatal(err)
			}
			item := opencodeHookItem(t, plan)
			if item.Action != "skip" {
				t.Fatalf("action = %q, want skip", item.Action)
			}
			if name == "other knowledge base" && !strings.Contains(item.CurrentState, "another knowledge base") {
				t.Fatalf("CurrentState = %q, want the other instance named", item.CurrentState)
			}
			before := targetState(t, target)
			if _, err := Apply(plan); err != nil {
				t.Fatal(err)
			}
			if after := targetState(t, target); after != before {
				t.Fatalf("foreign target disturbed:\nbefore %s\nafter %s", before, after)
			}
		})
	}
}

func targetState(t *testing.T, target string) string {
	t.Helper()
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		raw, err := os.Readlink(target)
		if err != nil {
			t.Fatal(err)
		}
		return "link:" + raw
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	return "file:" + string(body)
}

func TestOpencodePluginLinkSharedSettingsMergeOwners(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"z-oc\", \"a-oc\"]\n"+
		"[integrations.z-oc]\nprofile = \"opencode\"\n"+
		"[integrations.a-oc]\nprofile = \"opencode\"\n")
	plan, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for i := range plan.Items {
		if plan.Items[i].Kind == "hook-link" {
			count++
			if got := strings.Join(plan.Items[i].Owners, ","); got != "a-oc,z-oc" {
				t.Fatalf("shared plugin link owners = %q", got)
			}
		}
	}
	if count != 1 {
		t.Fatalf("shared settings produced %d hook-link items, want 1", count)
	}
}

func TestOpencodePluginLinkRepairsStaleInstanceLink(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	cfg := loadTestConfig(t, root, "integrations = [\"opencode\"]\n")
	target := filepath.Join(home, ".config", "opencode", "plugins", "regesto.ts")
	// A link left behind by an instance that has since moved or been removed:
	// regesto-shaped, but its knowledge base is no longer there.
	gone := filepath.Join(t.TempDir(), "gone-kb")
	stale := filepath.Join(gone, "adapters", "opencode", "plugins", "regesto.ts")
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
	item := opencodeHookItem(t, plan)
	if item.Action != "replace" || !strings.Contains(item.CurrentState, "no longer there") {
		t.Fatalf("stale link plan = %q / %q, want replace", item.Action, item.CurrentState)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "adapters", "opencode", "plugins", "regesto.ts")
	raw, err := os.Readlink(target)
	if err != nil || filepath.Clean(raw) != want {
		t.Fatalf("stale link repaired to %q (%v), want %q", raw, err, want)
	}
	second, err := Build(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := opencodeHookItem(t, second); got.Action != "current" {
		t.Fatalf("post-repair action = %q, want current", got.Action)
	}
}

func TestOpencodeProtocolCommandNeedsNoExecutableBit(t *testing.T) {
	root := t.TempDir()
	plugin := filepath.Join(root, "adapters", "opencode", "plugins", "regesto.ts")
	writeTestFile(t, plugin, "// launcher\n")
	command, event, err := protocolHookCommand(root, root, "opencode-chat-message-v1")
	if err != nil {
		t.Fatal(err)
	}
	if command != plugin || event != "prompt" {
		t.Fatalf("command=%q event=%q", command, event)
	}
}
