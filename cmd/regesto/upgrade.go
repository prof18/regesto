package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/manifest"
	"github.com/prof18/regesto/internal/version"
)

// runUpgrade refreshes the engine-owned files inside an instance after the
// engine itself has been replaced.
//
// It exists because `init` deliberately never overwrites — re-running it must
// not revert a skill someone edited on purpose — which leaves an instance
// holding whatever version scaffolded it, forever. Upgrading the binary alone
// updates none of the files it wrote.
func runUpgrade(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "report what would change and write nothing")
	force := fs.Bool("force", false, "also overwrite locally modified files, backing each one up first")
	if err := fs.Parse(args); err != nil {
		return err
	}

	engine, err := regesto.InstanceFiles()
	if err != nil {
		return err
	}
	m, err := manifest.Load(cfg.KBRoot)
	if err != nil {
		return err
	}

	from := m.Engine
	if from == "" {
		from = "unrecorded"
	}
	fmt.Printf("instance %s\nengine   %s → %s\n\n", cfg.KBRoot, from, version.Current())

	noteNewAgents(cfg)

	changes := manifest.Plan(cfg.KBRoot, engine, m)
	var written, removed, kept int
	// Start from what was already recorded, so a file this engine no longer
	// ships keeps its entry rather than silently losing its provenance.
	next := &manifest.Manifest{Engine: version.Current(), Written: time.Now(), Files: map[string]string{}}
	for p, h := range m.Files {
		next.Files[p] = h
	}

	for _, c := range changes {
		switch {
		case c.Status == manifest.Current:
			// Record the hash even when nothing is written: this is how an
			// instance that predates manifests adopts one without touching a
			// single file.
			next.Files[c.Path] = manifest.Sum(c.Body)

		case c.Status.Writable() || *force:
			if *dry {
				fmt.Printf("  would update  %-44s (%s)\n", c.Path, c.Status)
				written++
				continue
			}
			if c.Status == manifest.Modified || c.Status == manifest.Unknown {
				backup, err := backupFile(cfg.KBRoot, c.Path)
				if err != nil {
					return err
				}
				fmt.Printf("  backup        %s\n", backup)
			}
			if err := writeInstanceFile(cfg.KBRoot, c.Path, c.Body); err != nil {
				return err
			}
			fmt.Printf("  update        %-44s (%s)\n", c.Path, c.Status)
			next.Files[c.Path] = manifest.Sum(c.Body)
			written++

		default:
			// Modified or Unknown, without --force. Left alone and reported:
			// the user is the only one who knows whether their edit still
			// matters, and a silent overwrite is unrecoverable.
			fmt.Printf("  keep          %-44s (%s — edit yours or pass --force)\n", c.Path, c.Status)
			kept++
		}
	}

	// Files a previous engine wrote here and this one no longer ships. Left in
	// place, the installer keeps rendering and linking them, so a retired skill
	// goes on instructing agents after the engine disowned it.
	for _, c := range manifest.PlanRemovals(cfg.KBRoot, engine, m) {
		switch {
		case c.Status == manifest.Missing:
			// Already gone; just stop tracking it.
			delete(next.Files, c.Path)

		case c.Status.Removable() || *force:
			if *dry {
				fmt.Printf("  would remove  %-44s (%s)\n", c.Path, c.Status)
				removed++
				continue
			}
			if c.Status == manifest.WithdrawnEdited {
				backup, err := backupFile(cfg.KBRoot, c.Path)
				if err != nil {
					return err
				}
				fmt.Printf("  backup        %s\n", backup)
			}
			if err := removeInstanceFile(cfg.KBRoot, c.Path); err != nil {
				return err
			}
			fmt.Printf("  remove        %-44s (%s)\n", c.Path, c.Status)
			delete(next.Files, c.Path)
			removed++

		default:
			// Withdrawn but edited. The engine has no claim on someone's
			// changes, so it keeps both the file and its record.
			fmt.Printf("  keep          %-44s (%s — yours now; delete it yourself)\n", c.Path, c.Status)
			kept++
		}
	}

	if *dry {
		fmt.Printf("\ndry run — %d file(s) would change, %d would be removed, %d left alone.\n", written, removed, kept)
		fmt.Println("The adapters would then be reinstalled and the scheduled jobs repointed if needed.")
		fmt.Println("Nothing was written.")
		return nil
	}
	if err := manifest.Save(cfg.KBRoot, next); err != nil {
		return err
	}
	fmt.Printf("\n%d file(s) updated, %d removed, %d left alone.\n", written, removed, kept)

	// Refreshing the files is only half of it. The skills agents load are
	// rendered copies under .state/, the hook is registered in the agent's
	// settings, and the scheduled jobs name a binary by absolute path — none of
	// which change just because the sources did. Leaving those to the user meant
	// an upgrade that silently did nothing an agent could see.
	if err := reinstallAdapters(cfg); err != nil {
		return err
	}
	return repointSchedule(cfg)
}

// noteNewAgents flags a known agent that is present on this machine but
// missing from this instance's `agents` in config.toml — the case where a
// release adds an adapter for something the user already had installed
// before init ever ran its own detection.
//
// This only ever prints. config.toml is the one file upgrade never writes to
// — "everything machine- or person-specific lives here, never in the engine"
// — so silently appending to `agents` would be the one place upgrade crossed
// that line. The user adds it themselves if they want it managed.
func noteNewAgents(cfg *config.Config) {
	have := make(map[string]bool, len(cfg.IntegrationIDs()))
	for _, a := range cfg.IntegrationIDs() {
		have[a] = true
	}
	detected := adapters.Detect()
	if !cfg.UsesLegacyAgents() {
		var err error
		detected, err = adapters.DetectProfiles(cfg.KBRoot)
		if err != nil {
			fmt.Printf("note     could not detect configured integration profiles: %v\n\n", err)
			return
		}
	}
	var missing []string
	for _, a := range detected {
		if !have[a] {
			missing = append(missing, a)
		}
	}
	if len(missing) == 0 {
		return
	}
	vocabulary := "integrations"
	if cfg.UsesLegacyAgents() {
		vocabulary = "agents"
	}
	fmt.Printf("note     %s detected but not in %s = [...] in config.toml — add it yourself to have this instance manage it\n\n",
		strings.Join(missing, ", "), vocabulary)
}

// reinstallAdapters runs the instance's own install script, which was itself
// just refreshed — so an engine that changes how installing works takes effect
// on the same upgrade that ships the change.
func reinstallAdapters(cfg *config.Config) error {
	script := filepath.Join(cfg.KBRoot, "bin", "regesto-install")
	if _, err := os.Stat(script); err != nil {
		fmt.Printf("\nskipping adapter install — %s is not there\n", script)
		return nil
	}
	fmt.Println("\n── adapters ──")
	cmd := exec.Command(script)
	cmd.Dir = cfg.KBRoot
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bin/regesto-install failed: %w", err)
	}
	return nil
}

// repointSchedule rewrites the launchd jobs when they name an engine that is no
// longer the one serving this instance — the case that bites after switching
// from a local build to a released binary, where everything keeps working on the
// old engine and nothing says so.
//
// Jobs that were never installed are left alone: scheduling is opt-in. So are
// jobs belonging to a *different* instance. LaunchAgents live in one directory
// per user and the labels are fixed, so every instance on a machine sees the
// same two files — without this check, running upgrade in a second instance
// (or a throwaway one) silently steals the first one's schedule.
func repointSchedule(cfg *config.Config) error {
	want, err := config.ResolveEngine(cfg)
	if err != nil {
		return nil // no schedulable engine; `schedule install` will say why
	}
	// Compare what the paths resolve to, not the paths themselves. The engine is
	// normally reached through a stable symlink on PATH, and which name you
	// happened to invoke says nothing about which binary will run — comparing
	// literally would rewrite the jobs back and forth depending on how upgrade
	// was called.
	same := func(a, b string) bool {
		ra, erra := filepath.EvalSymlinks(a)
		rb, errb := filepath.EvalSymlinks(b)
		if erra != nil || errb != nil {
			return false // a path that no longer resolves is definitely stale
		}
		return ra == rb
	}
	plistBinary := regexp.MustCompile(`<string>([^<]*/regesto)</string>`)

	// The job is this instance's only if it runs against this instance's config.
	ours := []byte("<string>" + filepath.Join(cfg.KBRoot, config.FileName) + "</string>")

	stale := false
	installed := 0
	for _, j := range jobs(cfg) {
		body, err := os.ReadFile(plistPath(j.label))
		if err != nil {
			continue
		}
		if !bytes.Contains(body, ours) {
			fmt.Printf("\nleaving %s alone — it is scheduled against another instance\n", j.label)
			continue
		}
		installed++
		m := plistBinary.FindSubmatch(body)
		if m == nil || !same(string(m[1]), want) {
			stale = true
		}
	}
	if installed == 0 || !stale {
		return nil
	}
	fmt.Println("\n── scheduled jobs ──")
	fmt.Printf("they name a different engine; repointing at %s\n", want)
	lintHost := strings.TrimSpace(cfg.Section("roles")["lint"])
	return scheduleInstall(cfg, lintHost == "" || lintHost == cfg.Machine)
}

// backupFile copies a file aside before it is overwritten, next to the original
// so it is impossible to miss. Only reached under --force, which is the one path
// that can destroy someone's edit.
func backupFile(root, rel string) (string, error) {
	src := filepath.Join(root, filepath.FromSlash(rel))
	body, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	dest := src + ".regesto-backup." + time.Now().UTC().Format("20060102150405")
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

// removeInstanceFile deletes a withdrawn file and any directories it leaves
// empty, stopping at the instance root.
//
// The directory matters as much as the file: `regesto-install` treats every
// directory under adapters/skills/ as a skill, so a retired skill whose SKILL.md
// is gone but whose folder remains still gets rendered and linked — an empty
// skill offered to every agent. Only empty directories are removed, so anything
// the user put alongside keeps its home.
func removeInstanceFile(root, rel string) error {
	dest := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(dest)
	for dir != root && strings.HasPrefix(dir, root+string(filepath.Separator)) {
		// Fails harmlessly and stops the walk as soon as one is not empty.
		if err := os.Remove(dir); err != nil {
			return nil
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

func writeInstanceFile(root, rel string, body []byte) error {
	dest := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if regesto.Executable(rel) {
		mode = 0o755
	}
	if err := os.WriteFile(dest, body, mode); err != nil {
		return err
	}
	// WriteFile does not change the mode of a file that already exists.
	return os.Chmod(dest, mode)
}
