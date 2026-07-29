// Package harvest captures what agents wrote to their own native memory
// (PLAN 2.a, DESIGN §5).
//
// It is diff-based and read-only with respect to vendor files: a snapshot of
// each agent's memory tree is kept in .state/<machine>/, and anything new or
// changed since the last snapshot is copied into inbox/<agent>@<machine>/. That
// needs no cooperation from the agent — anything it wrote natively is picked up
// without it knowing this system exists.
//
// Two hazards shape the design:
//
//   - The prune race. Native memory is a cache the agent actively prunes, so a
//     note written *and* pruned between two snapshots never appears in any diff.
//     The interval is the loss window; regesto-write is the real guarantee.
//   - Never write a vendor file before harvesting it. Nothing here writes to
//     agent directories at all, which is what keeps that rule easy to hold.
package harvest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
)

// entry is one file as last seen. Content is hashed rather than stored: the
// snapshot stays small even when the tree is not.
type entry struct {
	Sha  string `json:"sha"`
	Size int64  `json:"size"`
}

type snapshot struct {
	Agent   string           `json:"agent"`
	Taken   string           `json:"taken"`
	Entries map[string]entry `json:"entries"`
}

// Result is what one agent's pass did, for the run summary.
type Result struct {
	Agent         string
	Scanned       int
	Captured      []string
	CapturedBytes int64
	Skipped       []string
	Note          string
}

// Run harvests every configured agent. It returns one Result per agent; an
// agent whose memory is absent on this machine yields a Result with a Note
// rather than an error, because a machine simply may not have that agent.
func Run(cfg *config.Config, dryRun bool) ([]Result, error) {
	if cfg.Machine == "" {
		return nil, fmt.Errorf("no machine name resolved; cannot name the inbox")
	}
	var results []Result
	for _, a := range adapters.For(cfg) {
		r, err := runAgent(cfg, a, dryRun)
		if err != nil {
			return results, err
		}
		results = append(results, r)
	}
	return results, nil
}

func runAgent(cfg *config.Config, a adapters.Agent, dryRun bool) (Result, error) {
	res := Result{Agent: a.Name}
	if a.MemoryGlob == "" {
		res.Note = "no memory location known"
		return res, nil
	}
	dirs, err := filepath.Glob(a.MemoryGlob)
	if err != nil {
		return res, fmt.Errorf("%s: bad memory glob %q: %w", a.Name, a.MemoryGlob, err)
	}
	if len(dirs) == 0 {
		res.Note = "not present on this machine"
		return res, nil
	}

	current := map[string]entry{}
	contents := map[string][]byte{}
	for _, dir := range dirs {
		// The key keeps the memory directory's own name, so Claude Code's
		// per-project stores stay distinguishable once flattened.
		prefix := filepath.Base(filepath.Dir(dir))
		if prefix == "." || prefix == string(filepath.Separator) {
			prefix = filepath.Base(dir)
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // an unreadable corner of a vendor dir must not abort the run
			}
			if d.IsDir() {
				// Vendor stores keep their own git and bulk subdirectories;
				// neither is a captured fact.
				if d.Name() == ".git" || d.Name() == "rollout_summaries" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".md") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return nil
			}
			key := filepath.ToSlash(filepath.Join(prefix, rel))
			res.Scanned++

			// Excluded by name: this is the deliberate "not content" control.
			// Size is only a proxy for that, and a poor one.
			for _, pat := range a.ExcludeGlobs {
				if ok, _ := filepath.Match(pat, d.Name()); ok {
					res.Skipped = append(res.Skipped,
						fmt.Sprintf("%s (excluded by [harvest_exclude].%s pattern %q)", key, a.Name, pat))
					return nil
				}
			}

			if a.MaxCaptureBytes > 0 && info.Size() > a.MaxCaptureBytes {
				res.Skipped = append(res.Skipped,
					fmt.Sprintf("%s (%d KB exceeds the %d KB guard — raise [harvest].max_capture_bytes if this is real content)",
						key, info.Size()/1024, a.MaxCaptureBytes/1024))
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			sum := sha256.Sum256(data)
			current[key] = entry{Sha: hex.EncodeToString(sum[:]), Size: info.Size()}
			contents[key] = data
			return nil
		})
		if err != nil {
			return res, err
		}
	}

	prev, err := loadSnapshot(cfg, a.Name)
	if err != nil {
		return res, err
	}

	var changed []string
	for key, e := range current {
		if old, ok := prev.Entries[key]; !ok || old.Sha != e.Sha {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)

	// A first run would otherwise dump the entire existing store into the
	// inbox. Record the baseline instead and capture only what changes after
	// it — the existing content is already covered by the 0.c migration.
	first := len(prev.Entries) == 0
	if first && len(changed) > 0 {
		res.Note = fmt.Sprintf("first run — recorded a baseline of %d file(s), captured none", len(changed))
		changed = nil
	}

	if !dryRun && len(changed) > 0 {
		stamp := time.Now().UTC().Format("20060102T150405Z")
		dest := filepath.Join(cfg.KBRoot, "inbox", a.Name+"@"+cfg.Machine, stamp)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return res, err
		}
		for _, key := range changed {
			name := strings.ReplaceAll(key, "/", "__")
			// Capture only what changed. The previous copy lives in .state/,
			// which is per-machine and never synced or committed, so the diff
			// costs local disk instead of repository and sync weight. A file
			// seen for the first time has no baseline and is captured whole.
			body, suffix := contents[key], ""
			if prevBlob, err := readBlob(cfg, a.Name, key); err == nil {
				if d, ok := unifiedDiff(prevBlob, contents[key], key); ok {
					body, suffix = d, ".diff"
				}
			}
			if err := os.WriteFile(filepath.Join(dest, name+suffix), body, 0o644); err != nil {
				return res, err
			}
			res.CapturedBytes += int64(len(body))
		}
	}
	res.Captured = changed

	// Refresh the diff baselines for everything currently tracked, not just
	// what changed. Writing whatever is missing or stale is self-healing: a
	// store baselined before blobs existed, or one whose .state was cleared,
	// recovers on the next pass instead of capturing whole files forever.
	if !dryRun {
		for key, e := range current {
			if prevBlob, err := readBlob(cfg, a.Name, key); err == nil {
				sum := sha256.Sum256(prevBlob)
				if hex.EncodeToString(sum[:]) == e.Sha {
					continue
				}
			}
			if err := writeBlob(cfg, a.Name, key, contents[key]); err != nil {
				return res, err
			}
		}
	}

	// The snapshot is written last, and only when captures were actually
	// persisted. If writing the inbox failed, the next run must see the same
	// files as still-unharvested rather than silently skipping them.
	if !dryRun {
		if err := saveSnapshot(cfg, a.Name, snapshot{
			Agent:   a.Name,
			Taken:   time.Now().UTC().Format(time.RFC3339),
			Entries: current,
		}); err != nil {
			return res, err
		}
	}
	return res, nil
}

func snapshotPath(cfg *config.Config, agent string) string {
	return filepath.Join(cfg.KBRoot, ".state", cfg.Machine, agent+".json")
}

func loadSnapshot(cfg *config.Config, agent string) (snapshot, error) {
	s := snapshot{Entries: map[string]entry{}}
	data, err := os.ReadFile(snapshotPath(cfg, agent))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		// A corrupt baseline must not wedge the loop; treat it as a first run.
		return snapshot{Entries: map[string]entry{}}, nil
	}
	if s.Entries == nil {
		s.Entries = map[string]entry{}
	}
	return s, nil
}

func saveSnapshot(cfg *config.Config, agent string, s snapshot) error {
	path := snapshotPath(cfg, agent)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
