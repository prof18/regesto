package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
	regestoinstall "github.com/prof18/regesto/internal/install"
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
	removals := manifest.PlanRemovals(cfg.KBRoot, engine, m)
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
	for _, c := range removals {
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
		fmt.Println("\n── adapters ──")
		sourceRoot, err := upgradePlanSourceRoot(cfg.KBRoot, changes, removals, *force)
		if err != nil {
			return err
		}
		defer os.RemoveAll(sourceRoot)
		plan, err := regestoinstall.Build(cfg, regestoinstall.Options{SourceRoot: sourceRoot})
		if err != nil {
			return err
		}
		printInstallPlan(plan, true)
		fmt.Printf("%d adapter change(s) planned. Scheduled jobs would be repointed if needed.\n", plan.Changes())
		fmt.Println("Nothing was written.")
		return nil
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
	// Record the engine checkpoint only after the refreshed artifacts installed
	// successfully. If host application fails, the next upgrade can safely
	// classify and retry the already-written instance files.
	if err := manifest.Save(cfg.KBRoot, next); err != nil {
		return err
	}
	return repointSchedule(cfg)
}

// upgradePlanSourceRoot constructs the adapter source tree that will exist
// immediately before a real upgrade calls reinstallAdapters. It starts with
// the instance's current sources, substitutes only files this upgrade is
// authorized to refresh, and removes only files it is authorized to retire.
// That distinction matters for edited sources: a dry-run must not claim it will
// render packaged bytes when the real upgrade will preserve the user's copy.
func upgradePlanSourceRoot(root string, changes, removals []manifest.Change, force bool) (string, error) {
	shadow, err := os.MkdirTemp("", "regesto-upgrade-plan-")
	if err != nil {
		return "", err
	}
	cleanup := func(cause error) (string, error) {
		if removeErr := os.RemoveAll(shadow); removeErr != nil {
			return "", fmt.Errorf("%v; clean temporary upgrade source: %w", cause, removeErr)
		}
		return "", cause
	}

	sourceAdapters := filepath.Join(root, "adapters")
	shadowAdapters := filepath.Join(shadow, "adapters")
	if _, err := os.Stat(sourceAdapters); err == nil {
		if err := os.CopyFS(shadowAdapters, os.DirFS(sourceAdapters)); err != nil {
			return cleanup(fmt.Errorf("copy adapter sources for upgrade plan: %w", err))
		}
		if err := filepath.WalkDir(sourceAdapters, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(sourceAdapters, path)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return os.Chmod(filepath.Join(shadowAdapters, relative), info.Mode().Perm())
		}); err != nil {
			return cleanup(fmt.Errorf("preserve adapter source modes for upgrade plan: %w", err))
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(shadowAdapters, 0o755); err != nil {
			return cleanup(err)
		}
	} else {
		return cleanup(err)
	}

	for _, change := range changes {
		if !strings.HasPrefix(change.Path, "adapters/") || (!change.Status.Writable() && !force) {
			continue
		}
		if err := writeInstanceFile(shadow, change.Path, change.Body); err != nil {
			return cleanup(fmt.Errorf("stage upgraded adapter source %s: %w", change.Path, err))
		}
	}
	for _, removal := range removals {
		if !strings.HasPrefix(removal.Path, "adapters/") || (!removal.Status.Removable() && !force) {
			continue
		}
		if err := removeInstanceFile(shadow, removal.Path); err != nil {
			return cleanup(fmt.Errorf("stage retired adapter source %s: %w", removal.Path, err))
		}
	}
	return shadow, nil
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

// reinstallAdapters calls the same Go planner/applicator as `regesto install`.
// Upgrade has already refreshed the instance templates, so the plan observes
// the just-installed artifact sources without shell config parsing.
func reinstallAdapters(cfg *config.Config) error {
	fmt.Println("\n── adapters ──")
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{})
	if err != nil {
		return err
	}
	printInstallPlan(plan, false)
	result, err := regestoinstall.Apply(plan)
	if err != nil {
		return err
	}
	fmt.Printf("%d adapter change(s) applied.\n", result.Applied)
	for _, path := range result.Backups {
		fmt.Printf("  backup %s\n", path)
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
	instance, name, err := openInstanceRoot(root, rel)
	if err != nil {
		return "", err
	}
	defer instance.Close()
	info, err := instance.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("refuse to back up non-regular instance file %s", rel)
	}
	body, err := instance.ReadFile(name)
	if err != nil {
		return "", err
	}
	base := name + ".regesto-backup." + time.Now().UTC().Format("20060102T150405.000000000Z")
	for attempt := 0; attempt < 100; attempt++ {
		candidate := base
		if attempt > 0 {
			candidate = fmt.Sprintf("%s.%d", base, attempt)
		}
		backup, err := instance.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := backup.Chmod(info.Mode().Perm()); err != nil {
			backup.Close()
			instance.Remove(candidate)
			return "", err
		}
		if _, err := backup.Write(body); err != nil {
			backup.Close()
			instance.Remove(candidate)
			return "", err
		}
		if err := backup.Sync(); err != nil {
			backup.Close()
			instance.Remove(candidate)
			return "", err
		}
		if err := backup.Close(); err != nil {
			instance.Remove(candidate)
			return "", err
		}
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		return filepath.Join(absoluteRoot, candidate), nil
	}
	return "", fmt.Errorf("could not create a unique backup beside %s", rel)
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
	instance, name, err := openInstanceRoot(root, rel)
	if err != nil {
		return err
	}
	defer instance.Close()
	if err := instance.Remove(name); err != nil && !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(name)
	for dir != "." {
		// Fails harmlessly and stops the walk as soon as one is not empty.
		if err := instance.Remove(dir); err != nil {
			return nil
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

func writeInstanceFile(root, rel string, body []byte) error {
	instance, name, err := openInstanceRoot(root, rel)
	if err != nil {
		return err
	}
	defer instance.Close()
	parent := filepath.Dir(name)
	if parent != "." {
		if err := instance.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	mode := os.FileMode(0o644)
	if regesto.Executable(rel) {
		mode = 0o755
	}
	tmp, tmpName, err := createInstanceTemp(instance, parent, mode)
	if err != nil {
		return err
	}
	defer instance.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return instance.Rename(tmpName, name)
}

func openInstanceRoot(root, rel string) (*os.Root, string, error) {
	if !fs.ValidPath(filepath.ToSlash(rel)) {
		return nil, "", fmt.Errorf("invalid instance path %q", rel)
	}
	instance, err := os.OpenRoot(root)
	if err != nil {
		return nil, "", err
	}
	return instance, filepath.FromSlash(rel), nil
}

func createInstanceTemp(instance *os.Root, parent string, mode os.FileMode) (*os.File, string, error) {
	base := fmt.Sprintf(".regesto-upgrade-%d", time.Now().UnixNano())
	for attempt := 0; attempt < 100; attempt++ {
		candidate := filepath.Join(parent, fmt.Sprintf("%s-%d", base, attempt))
		file, err := instance.OpenFile(candidate, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if os.IsExist(err) {
			continue
		}
		if err == nil {
			if chmodErr := file.Chmod(mode); chmodErr != nil {
				file.Close()
				instance.Remove(candidate)
				return nil, "", chmodErr
			}
		}
		return file, candidate, err
	}
	return nil, "", fmt.Errorf("could not create a temporary instance file in %s", parent)
}
