package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	regesto "github.com/prof18/regesto"
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

	changes := manifest.Plan(cfg.KBRoot, engine, m)
	var written, kept int
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

	if *dry {
		fmt.Printf("\ndry run — %d file(s) would change, %d left alone. Nothing was written.\n", written, kept)
		return nil
	}
	if err := manifest.Save(cfg.KBRoot, next); err != nil {
		return err
	}
	fmt.Printf("\n%d file(s) updated, %d left alone. Manifest at %s\n", written, kept, manifest.FileName)
	if written > 0 {
		// The skills agents actually load are rendered copies under .state/,
		// so updating the sources here changes nothing until install re-renders.
		fmt.Println("Re-run bin/regesto-install so the agents pick up the new skills and instructions.")
	}
	return nil
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
