package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/prof18/regesto/internal/adapters"
)

const opencodePluginRelative = "adapters/opencode/plugins/regesto.ts"
const piExtensionRelative = "adapters/pi/extensions/regesto.ts"

// launcherRelatives are the adapter files whose presence in a host's
// auto-discovered directory is the whole hook registration. Hosts that work
// this way get a linked launcher instead of a settings edit.
var launcherRelatives = []string{opencodePluginRelative, piExtensionRelative}

// planOpencodePluginLink installs the OpenCode plugin as a symlink into the
// directory OpenCode auto-discovers. OpenCode has no settings-file hook
// registration, so the linked launcher file is the whole registration: it
// self-locates the instance the same way the bin/ shims and hook wrappers do,
// which is why this is a link rather than a rendered copy.
func planOpencodePluginLink(p *Plan, agent adapters.Agent, hook adapters.Hook, sourceRoot string) (Item, error) {
	return planLauncherLink(p, agent, hook, sourceRoot, opencodePluginRelative)
}

// planPiExtensionLink installs the pi extension as a symlink into
// ~/.pi/agent/extensions/, the directory pi auto-discovers global extensions
// from. Like OpenCode, pi has no settings-file registration for this: the
// linked launcher is the registration.
func planPiExtensionLink(p *Plan, agent adapters.Agent, hook adapters.Hook, sourceRoot string) (Item, error) {
	return planLauncherLink(p, agent, hook, sourceRoot, piExtensionRelative)
}

// planLauncherLink plans the symlink of one adapter launcher file into a
// host-owned auto-discovered directory declared as the hook's settings target.
func planLauncherLink(p *Plan, agent adapters.Agent, hook adapters.Hook, sourceRoot, relative string) (Item, error) {
	if hook.Settings == "" {
		return Item{}, fmt.Errorf("integration %q: launcher registrar has no target", agent.Name)
	}
	if err := requireLauncherFile(sourceRoot, relative); err != nil {
		return Item{}, fmt.Errorf("integration %q: %w", agent.Name, err)
	}
	canonical, err := canonicalLinkPath(hook.Settings)
	if err != nil {
		return Item{}, err
	}
	want := filepath.Join(p.KBRoot, filepath.FromSlash(relative))
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
		item.Action, item.CurrentState, item.DryRun = "create", "launcher not linked yet", "link the Regesto launcher"
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
		item.Action, item.CurrentState, item.DryRun = "current", "Regesto's launcher is already linked", "no change"
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
// launcher. Every knowledge base keeps each launcher at its adapter-relative
// path inside the instance.
func OtherAdapterKnowledgeBase(linkTarget string) string {
	for _, relative := range launcherRelatives {
		marker := string(filepath.Separator) + filepath.FromSlash(relative)
		if i := strings.Index(linkTarget, marker); i > 0 {
			return linkTarget[:i]
		}
	}
	return ""
}

// requireLauncherFile reports whether the adapter source tree ships the
// launcher file the link points at. Unlike the shell hook wrappers it is
// loaded, not executed, so a regular file is enough and no executable bit is
// required.
func requireLauncherFile(sourceRoot, relative string) error {
	source := filepath.Join(sourceRoot, filepath.FromSlash(relative))
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("launcher %s is unavailable: %w", source, err)
	}
	if info.IsDir() {
		return fmt.Errorf("launcher %s is a directory", source)
	}
	return nil
}
