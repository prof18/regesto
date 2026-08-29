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
	"io"
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
	Agent             string                    `json:"agent"`
	Taken             string                    `json:"taken"`
	Entries           map[string]entry          `json:"entries"`
	LegacySource      string                    `json:"legacy_source,omitempty"`
	LegacyInitialized bool                      `json:"legacy_initialized,omitempty"`
	Sources           map[string]sourceSnapshot `json:"sources,omitempty"`
	Observed          map[string]entry          `json:"observed,omitempty"`
}

type sourceSnapshot struct {
	Initialized bool             `json:"initialized"`
	Entries     map[string]entry `json:"entries"`
}

// Result is what one agent's pass did, for the run summary.
type Result struct {
	Agent         string
	SourceID      string
	Kind          string
	Location      string
	Scanned       int
	Captured      []string
	CapturedBytes int64
	Skipped       []string
	Note          string
}

// Run harvests every configured integration. It returns one Result per memory
// source; an absent or unsupported source yields an explicit Note rather than
// disappearing, because a machine simply may not expose every declared store.
func Run(cfg *config.Config, dryRun bool) ([]Result, error) {
	if cfg.Machine == "" {
		return nil, fmt.Errorf("no machine name resolved; cannot name the inbox")
	}
	if !safeNamespaceComponent(cfg.Machine) {
		return nil, fmt.Errorf("machine name %q is not a safe inbox/state path component", cfg.Machine)
	}
	resolved, err := adapters.Resolve(cfg)
	if err != nil {
		return nil, err
	}
	kbFS, canonicalKB, err := openHarvestRoot(cfg.KBRoot)
	if err != nil {
		return nil, fmt.Errorf("open knowledge-base root for harvesting: %w", err)
	}
	defer kbFS.Close()
	if err := validateHarvestOutputRoots(kbFS); err != nil {
		return nil, err
	}
	for _, a := range resolved {
		if !safeNamespaceComponent(a.Name) {
			return nil, fmt.Errorf("integration name %q is not a safe inbox/state path component", a.Name)
		}
		if err := preflightMemorySources(canonicalKB, a); err != nil {
			return nil, err
		}
	}
	var results []Result
	for _, a := range resolved {
		r, err := runAgent(kbFS, cfg, a, dryRun)
		if err != nil {
			return results, err
		}
		results = append(results, r...)
	}
	return results, nil
}

func preflightMemorySources(kbRoot string, a adapters.Agent) error {
	for _, source := range a.MemorySources {
		if source.Kind != "markdown-glob-v1" {
			continue
		}
		matches, err := filepath.Glob(source.Location)
		if err != nil {
			return fmt.Errorf("%s: bad markdown-glob-v1 location %q: %w", a.Name, source.Location, err)
		}
		for _, match := range matches {
			resolved, err := filepath.EvalSymlinks(match)
			if err != nil {
				continue
			}
			resolved, err = filepath.Abs(resolved)
			if err != nil {
				return err
			}
			if pathsOverlap(kbRoot, resolved) {
				return fmt.Errorf("integration %q memory source %q overlaps knowledge-base root %s; refuse to harvest Regesto's own state", a.Name, match, kbRoot)
			}
		}
	}
	return nil
}

func pathsOverlap(first, second string) bool {
	return pathWithin(first, second) || pathWithin(second, first)
}

func pathWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func safeNamespaceComponent(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value
}

func runAgent(kbFS *os.Root, cfg *config.Config, a adapters.Agent, dryRun bool) ([]Result, error) {
	if len(a.MemorySources) == 0 {
		return []Result{{Agent: a.Name, Note: "no memory sources declared"}}, nil
	}
	state, err := loadSnapshot(kbFS, cfg, a.Name)
	if err != nil {
		return nil, err
	}
	var firstMarkdown string
	for _, source := range a.MemorySources {
		if source.Kind == "markdown-glob-v1" {
			firstMarkdown = memorySourceID(source)
			break
		}
	}
	if state.LegacySource == "" {
		state.LegacySource = firstMarkdown
	}

	publishedFiles := map[string]bool{}
	observedFiles := map[string]entry{}
	var results []Result
	touched := false
	for _, source := range a.MemorySources {
		res := Result{Agent: a.Name, SourceID: memorySourceID(source), Kind: source.Kind, Location: source.Location}
		switch source.Kind {
		case "none":
			res.Note = "memory kind none — harvesting disabled"
			results = append(results, res)
			continue
		case "markdown-glob-v1":
			legacy := res.SourceID == state.LegacySource
			res, sourceTouched, err := runMarkdownSource(kbFS, cfg, a, source, res.SourceID, legacy, &state, publishedFiles, observedFiles, dryRun)
			if err != nil {
				return results, err
			}
			touched = touched || sourceTouched
			results = append(results, res)
		default:
			res.Note = fmt.Sprintf("unsupported memory kind %q — skipped", source.Kind)
			results = append(results, res)
		}
	}
	if !dryRun && touched {
		state.Agent = a.Name
		state.Taken = time.Now().UTC().Format(time.RFC3339)
		if state.Observed == nil {
			state.Observed = map[string]entry{}
		}
		for id, observed := range observedFiles {
			state.Observed[id] = observed
		}
		if err := saveSnapshot(kbFS, cfg, a.Name, state); err != nil {
			return results, err
		}
	}
	return results, nil
}

func memorySourceID(source adapters.MemorySource) string {
	sum := sha256.Sum256([]byte(source.Kind + "\x00" + filepath.Clean(source.Location)))
	return source.Kind + "-" + hex.EncodeToString(sum[:8])
}

func runMarkdownSource(kbFS *os.Root, cfg *config.Config, a adapters.Agent, source adapters.MemorySource, sourceID string, legacy bool, state *snapshot, publishedFiles map[string]bool, observedFiles map[string]entry, dryRun bool) (Result, bool, error) {
	res := Result{Agent: a.Name, SourceID: sourceID, Kind: source.Kind, Location: source.Location}
	dirs, err := filepath.Glob(source.Location)
	if err != nil {
		return res, false, fmt.Errorf("%s: bad markdown-glob-v1 location %q: %w", a.Name, source.Location, err)
	}
	sort.Strings(dirs)
	if len(dirs) == 0 {
		res.Note = fmt.Sprintf("markdown-glob-v1 source not present: %s", source.Location)
		return res, false, nil
	}

	current := map[string]entry{}
	contents := map[string][]byte{}
	keyOrigins := map[string]string{}
	flatKeys := map[string]string{}
	for _, dir := range dirs {
		walkRoot, resolveErr := filepath.EvalSymlinks(dir)
		if resolveErr != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (memory source root cannot be resolved: %v)", dir, resolveErr))
			continue
		}
		rootInfo, err := os.Stat(walkRoot)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (memory source root unreadable: %v)", dir, err))
			continue
		}
		rootIsFile := rootInfo.Mode().IsRegular()
		// The key keeps the memory directory's own name, so Claude Code's
		// per-project stores stay distinguishable once flattened.
		prefix := filepath.Base(filepath.Dir(dir))
		if prefix == "." || prefix == string(filepath.Separator) {
			prefix = filepath.Base(dir)
		}
		err = filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // an unreadable corner of a vendor dir must not abort the run
			}
			if d.Type()&os.ModeSymlink != 0 {
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s (symlinked vendor entry)", path))
				return nil
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
			if !info.Mode().IsRegular() {
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s (non-regular vendor entry)", path))
				return nil
			}
			rel, err := filepath.Rel(walkRoot, path)
			if err != nil {
				return nil
			}
			if rootIsFile && rel == "." {
				rel = filepath.Base(dir)
			}
			key := filepath.ToSlash(filepath.Join(prefix, rel))
			origin, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil
			}
			origin, err = filepath.Abs(origin)
			if err != nil {
				return nil
			}
			origin = filepath.Clean(origin)
			if previous, exists := keyOrigins[key]; exists && previous != origin {
				return fmt.Errorf("memory source %s maps both %s and %s to snapshot key %q", source.Location, previous, origin, key)
			}
			keyOrigins[key] = origin
			flat := strings.ReplaceAll(key, "/", "__")
			if previous, exists := flatKeys[flat]; exists && previous != key {
				return fmt.Errorf("memory source %s maps snapshot keys %q and %q to the same capture name", source.Location, previous, key)
			}
			flatKeys[flat] = key
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
			data, err := readStableRegularFile(path, info)
			if err != nil {
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s (changed or became unsafe before capture)", path))
				return nil
			}
			sum := sha256.Sum256(data)
			current[key] = entry{Sha: hex.EncodeToString(sum[:]), Size: info.Size()}
			observedFiles[memoryOriginID(origin)] = current[key]
			contents[key] = data
			return nil
		})
		if err != nil {
			return res, false, err
		}
	}

	previous := sourceSnapshot{Entries: map[string]entry{}}
	if legacy {
		previous = sourceSnapshot{Initialized: state.LegacyInitialized, Entries: state.Entries}
	} else if saved, ok := state.Sources[sourceID]; ok {
		previous = saved
	}
	if previous.Entries == nil {
		previous.Entries = map[string]entry{}
	}

	var changed []string
	for key, e := range current {
		if old, ok := previous.Entries[key]; !ok || old.Sha != e.Sha {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)

	// A first run would otherwise dump the entire existing store into the
	// inbox. Record the baseline instead and capture only what changes after
	// it — the existing content is already covered by the 0.c migration.
	first := !previous.Initialized
	if first && len(changed) > 0 {
		res.Note = fmt.Sprintf("first run — recorded a baseline of %d file(s), captured none", len(changed))
		changed = nil
	}
	var publish []string
	for _, key := range changed {
		origin := keyOrigins[key]
		if previous, ok := state.Observed[memoryOriginID(origin)]; ok && previous.Sha == current[key].Sha {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (content already represented by an overlapping memory source)", key))
			continue
		}
		if publishedFiles[origin] {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s (change already captured from an overlapping memory source)", key))
			continue
		}
		publishedFiles[origin] = true
		publish = append(publish, key)
	}
	changed = publish

	if !dryRun && len(changed) > 0 {
		stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
		dest := filepath.Join("inbox", a.Name+"@"+cfg.Machine, stamp)
		for _, key := range changed {
			name := strings.ReplaceAll(key, "/", "__")
			// Capture only what changed. The previous copy lives in .state/,
			// which is per-machine and never synced or committed, so the diff
			// costs local disk instead of repository and sync weight. A file
			// seen for the first time has no baseline and is captured whole.
			body, suffix := contents[key], ""
			if prevBlob, err := readSourceBlob(kbFS, cfg, a.Name, sourceID, legacy, key); err == nil {
				if d, ok := unifiedDiff(prevBlob, contents[key], key); ok {
					body, suffix = d, ".diff"
				}
			}
			if !legacy {
				name = sourceID + "__" + name
			}
			if err := writeRootExclusive(kbFS, filepath.Join(dest, name+suffix), body, 0o644); err != nil {
				return res, false, err
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
			if prevBlob, err := readSourceBlob(kbFS, cfg, a.Name, sourceID, legacy, key); err == nil {
				sum := sha256.Sum256(prevBlob)
				if hex.EncodeToString(sum[:]) == e.Sha {
					continue
				}
			}
			if err := writeSourceBlob(kbFS, cfg, a.Name, sourceID, legacy, key, contents[key]); err != nil {
				return res, false, err
			}
		}
	}

	if !dryRun {
		updated := sourceSnapshot{Initialized: true, Entries: current}
		if legacy {
			state.LegacyInitialized = true
			state.Entries = current
		} else {
			if state.Sources == nil {
				state.Sources = map[string]sourceSnapshot{}
			}
			state.Sources[sourceID] = updated
		}
	}
	return res, true, nil
}

func readStableRegularFile(path string, before os.FileInfo) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, fmt.Errorf("file identity changed before read")
	}
	return io.ReadAll(file)
}

func memoryOriginID(origin string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(origin)))
	return hex.EncodeToString(sum[:16])
}

func loadSnapshot(kbFS *os.Root, cfg *config.Config, agent string) (snapshot, error) {
	s := snapshot{Entries: map[string]entry{}, Sources: map[string]sourceSnapshot{}, Observed: map[string]entry{}}
	data, err := readRootFile(kbFS, snapshotRelativePath(cfg, agent))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		// A corrupt baseline must not wedge the loop; treat it as a first run.
		return snapshot{Entries: map[string]entry{}, Sources: map[string]sourceSnapshot{}, Observed: map[string]entry{}}, nil
	}
	if s.Entries == nil {
		s.Entries = map[string]entry{}
	}
	if s.Sources == nil {
		s.Sources = map[string]sourceSnapshot{}
	}
	if s.Observed == nil {
		s.Observed = map[string]entry{}
	}
	// Any readable pre-M8 snapshot represents a completed legacy baseline,
	// including an empty store. The added flag prevents a later first file from
	// being silently treated as another first run.
	if !s.LegacyInitialized {
		s.LegacyInitialized = true
	}
	return s, nil
}

func saveSnapshot(kbFS *os.Root, cfg *config.Config, agent string, s snapshot) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeRootAtomic(kbFS, snapshotRelativePath(cfg, agent), data, 0o644)
}

func snapshotRelativePath(cfg *config.Config, agent string) string {
	return filepath.Join(".state", cfg.Machine, agent+".json")
}
