package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/prof18/regesto/internal/adapters"
)

const opencodePluginRelative = "adapters/opencode/plugins/regesto.ts"

// planOpencodePluginLink installs the OpenCode plugin as a symlink into the
// directory OpenCode auto-discovers. OpenCode has no settings-file hook
// registration, so the linked launcher file is the whole registration: it
// self-locates the instance the same way the bin/ shims and hook wrappers do,
// which is why this is a link rather than a rendered copy.
func planOpencodePluginLink(p *Plan, agent adapters.Agent, hook adapters.Hook, sourceRoot string) (Item, error) {
	if hook.Settings == "" {
		return Item{}, fmt.Errorf("integration %q: OpenCode registrar has no plugin target", agent.Name)
	}
	if err := requirePluginLauncher(sourceRoot); err != nil {
		return Item{}, fmt.Errorf("integration %q: %w", agent.Name, err)
	}
	canonical, err := canonicalLinkPath(hook.Settings)
	if err != nil {
		return Item{}, err
	}
	want := filepath.Join(p.KBRoot, filepath.FromSlash(opencodePluginRelative))
	item := Item{
		ID:              "hook:" + canonical,
		Kind:            "hook-link",
		Owners:          []string{agent.Name},
		DeclaredTargets: []string{hook.Settings},
		CanonicalTarget: canonical,
		IntendedState:   "link to " + want,
		BackupAction:    "none",
		linkTarget:      want,
	}
	info, err := os.Lstat(canonical)
	if os.IsNotExist(err) {
		item.Action, item.CurrentState, item.DryRun = "create", "plugin not linked yet", "link the Regesto plugin"
		return item, nil
	}
	if err != nil {
		return Item{}, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		item.Action, item.CurrentState, item.DryRun = "skip", "another file is already there; left alone", "no change; remove that file to let Regesto create the link"
		return item, nil
	}
	resolved, resolveErr := filepath.EvalSymlinks(canonical)
	wantResolved, wantErr := filepath.EvalSymlinks(want)
	if resolveErr == nil && wantErr == nil && resolved == wantResolved {
		item.Action, item.CurrentState, item.DryRun = "current", "Regesto's plugin is already linked", "no change"
		return item, nil
	}
	raw, err := os.Readlink(canonical)
	if err != nil {
		return Item{}, err
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(filepath.Dir(canonical), raw)
	}
	raw = filepath.Clean(raw)
	if other := OtherAdapterKnowledgeBase(raw); other != "" {
		if _, statErr := os.Stat(filepath.Join(other, "config.toml")); statErr == nil {
			item.Action, item.CurrentState, item.DryRun = "skip", "used by another knowledge base ("+other+"); left alone", "no change"
			return item, nil
		}
		// The link carries this adapter's path shape but its knowledge base
		// is no longer there — the instance moved or was removed. Repairing
		// it is the same ownership rule skill links apply to stale copies:
		// regesto-shaped and dead means this install owns it again.
		item.Action, item.CurrentState, item.DryRun = "replace", "link points at a knowledge base that is no longer there ("+other+")", "update the link"
		item.before = []byte(raw)
		return item, nil
	}
	item.Action, item.CurrentState, item.DryRun = "skip", "already links to "+raw+"; left alone", "no change"
	return item, nil
}

// OtherAdapterKnowledgeBase returns the root of the Regesto knowledge base an
// adapter launcher link points into, or "" when the target is not a Regesto
// OpenCode plugin. Every knowledge base keeps the plugin at
// adapters/opencode/plugins/ inside the instance.
func OtherAdapterKnowledgeBase(linkTarget string) string {
	marker := string(filepath.Separator) + filepath.FromSlash(opencodePluginRelative)
	if i := strings.Index(linkTarget, marker); i > 0 {
		return linkTarget[:i]
	}
	return ""
}

// requirePluginLauncher reports whether the adapter source tree ships the
// plugin file the link points at. Unlike the shell hook wrappers it is loaded,
// not executed, so a regular file is enough and no executable bit is required.
func requirePluginLauncher(sourceRoot string) error {
	source := filepath.Join(sourceRoot, filepath.FromSlash(opencodePluginRelative))
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("plugin launcher %s is unavailable: %w", source, err)
	}
	if info.IsDir() {
		return fmt.Errorf("plugin launcher %s is a directory", source)
	}
	return nil
}
