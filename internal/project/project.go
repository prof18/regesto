// Package project maps a working directory to its canonical project name
// (PLAN 0.2 / 1.c). Claude Code's native memory directory name derives from the
// absolute repo path, so clones and machines produce different keys for one
// project — but the remote URL is the one thing every checkout shares. The
// hand-kept map in config.toml covers remote-less repos and basename
// collisions.
package project

import (
	"os/exec"
	"path/filepath"
	"strings"

	"regesto/internal/config"
)

// Resolution records how a name was derived, so a hook or `--debug` run can
// explain itself instead of silently guessing wrong.
type Resolution struct {
	Name string
	// How is one of: "flag", "config-path", "git-remote", "dir-basename".
	How string
	// Mapped reports whether config.toml's [projects] table rewrote the
	// derived name.
	Mapped bool
}

// Resolve determines the canonical project name for dir. It never fails: a
// directory that is not a git repo still yields a name from its basename, which
// is what makes the hook safe to run anywhere.
func Resolve(cfg *config.Config, dir string) Resolution {
	top := gitTopLevel(dir)
	if top == "" {
		top = dir
	}

	// An absolute-path key in [projects] is the most specific override, and the
	// only way to disambiguate two checkouts that share a remote basename.
	if abs, err := filepath.Abs(top); err == nil {
		if mapped, ok := cfg.Projects[abs]; ok {
			return Resolution{Name: mapped, How: "config-path", Mapped: true}
		}
	}

	if remote := gitRemoteBasename(top); remote != "" {
		name, mapped := canonical(cfg, remote)
		return Resolution{Name: name, How: "git-remote", Mapped: mapped}
	}

	name, mapped := canonical(cfg, filepath.Base(top))
	return Resolution{Name: name, How: "dir-basename", Mapped: mapped}
}

// canonical applies config.toml's [projects] table, which is what collapses
// feed-flow-2 onto feed-flow.
func canonical(cfg *config.Config, derived string) (string, bool) {
	if mapped, ok := cfg.Projects[derived]; ok {
		return mapped, true
	}
	return derived, false
}

// gitTopLevel returns the outermost working tree containing dir.
//
// A submodule is its own repository with its own remote, so a vendored
// dependency would otherwise resolve to the dependency's name rather than the
// project that vendors it — editing difftray-mobile's vendor/difftray would
// inject difftray's facts and file new ones under difftray, splitting the
// project in two on every visit. Climbing to the superproject follows what the
// person is actually working on. The loop handles submodules within submodules.
func gitTopLevel(dir string) string {
	top := gitOutput(dir, "rev-parse", "--show-toplevel")
	if top == "" {
		return ""
	}
	for i := 0; i < 10; i++ {
		super := gitOutput(top, "rev-parse", "--show-superproject-working-tree")
		if super == "" {
			break
		}
		top = super
	}
	return top
}

// gitRemoteBasename returns the origin URL's basename with any .git suffix
// removed — the stable identity every clone of a project shares.
func gitRemoteBasename(dir string) string {
	url := gitOutput(dir, "remote", "get-url", "origin")
	if url == "" {
		return ""
	}
	url = strings.TrimSuffix(strings.TrimRight(url, "/"), ".git")
	// Handle both scp-style (git@host:owner/repo) and URL forms.
	if i := strings.LastIndexAny(url, "/:"); i >= 0 {
		url = url[i+1:]
	}
	return strings.TrimSuffix(url, ".git")
}

func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
