package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"regesto/internal/facts"
)

// ConflictResolution is one decided sync conflict, for the run summary.
type ConflictResolution struct {
	ConflictPath string
	BasePath     string
	// Winner and Loser are paths; Winner is where the surviving content ends
	// up, which is always the base path.
	Kept     string
	Archived string
	Message  string
	// NeedsHuman marks a conflict lint will not decide.
	NeedsHuman bool
}

// ResolveConflicts decides sync-conflict copies (PLAN 2.d).
//
// The rule is the same one reconciliation uses: newer `modified` wins, and the
// loser is archived rather than deleted, so a wrong call is recoverable. This is
// the one place lint overwrites a file's content rather than a single field,
// which is exactly why every resolution is reported.
//
// Two cases are left for a human. A conflict copy that does not parse cannot be
// compared, and a conflict whose original has been deleted is a delete-versus-edit
// that only a person can settle — one machine decided the claim should go while
// another was still editing it.
func ResolveConflicts(kbRoot string, conflicts []facts.Conflict, apply bool, now time.Time) ([]ConflictResolution, error) {
	var out []ConflictResolution

	for _, c := range conflicts {
		res := ConflictResolution{ConflictPath: c.ConflictPath, BasePath: c.BasePath}

		if c.BasePath == "" {
			res.NeedsHuman = true
			res.Message = "the original is gone — delete on one machine against an edit on another; " +
				"decide by hand, then rename the copy back or archive it"
			out = append(out, res)
			continue
		}

		conflictFull := filepath.Join(kbRoot, c.ConflictPath)
		baseFull := filepath.Join(kbRoot, c.BasePath)

		conflictFact, cErr := facts.ParseFile(conflictFull)
		baseFact, bErr := facts.ParseFile(baseFull)
		if cErr != nil || bErr != nil {
			res.NeedsHuman = true
			res.Message = "one of the two copies does not parse, so they cannot be compared by `modified`"
			out = append(out, res)
			continue
		}

		conflictTime, cTimeErr := time.Parse(time.RFC3339, conflictFact.Modified)
		baseTime, bTimeErr := time.Parse(time.RFC3339, baseFact.Modified)
		if cTimeErr != nil || bTimeErr != nil {
			res.NeedsHuman = true
			res.Message = "one copy has an unparseable `modified`, so recency cannot decide it"
			out = append(out, res)
			continue
		}

		// Identical content is not a conflict worth a decision — the copy is
		// redundant and simply archived.
		conflictRaw, err1 := os.ReadFile(conflictFull)
		baseRaw, err2 := os.ReadFile(baseFull)
		identical := err1 == nil && err2 == nil && string(conflictRaw) == string(baseRaw)

		switch {
		case identical:
			res.Kept = c.BasePath
			res.Message = "copies are identical; the redundant one is archived"
		case conflictTime.After(baseTime):
			res.Kept = c.BasePath
			res.Message = fmt.Sprintf("the conflict copy is newer (%s > %s) and becomes the file; "+
				"the previous content is archived", conflictFact.Modified, baseFact.Modified)
		default:
			res.Kept = c.BasePath
			res.Message = fmt.Sprintf("the existing file is newer or equal (%s >= %s) and stands; "+
				"the conflict copy is archived", baseFact.Modified, conflictFact.Modified)
		}

		if !apply {
			out = append(out, res)
			continue
		}

		archiveDir := filepath.Join(kbRoot, "archive", "inbox", now.UTC().Format("2006-01-02"), "sync-conflicts")
		if err := os.MkdirAll(archiveDir, 0o755); err != nil {
			return out, err
		}

		if !identical && conflictTime.After(baseTime) {
			// The conflict copy wins: archive what the file currently holds,
			// then promote the copy into its place.
			losingName := filepath.Base(c.BasePath) + ".superseded-" + now.UTC().Format("20060102T150405Z")
			if err := os.Rename(baseFull, filepath.Join(archiveDir, losingName)); err != nil {
				return out, err
			}
			if err := os.Rename(conflictFull, baseFull); err != nil {
				return out, err
			}
			res.Archived = filepath.ToSlash(filepath.Join("archive", "inbox",
				now.UTC().Format("2006-01-02"), "sync-conflicts", losingName))
		} else {
			// The file stands; the copy is archived untouched.
			losingName := filepath.Base(c.ConflictPath)
			if err := os.Rename(conflictFull, filepath.Join(archiveDir, losingName)); err != nil {
				return out, err
			}
			res.Archived = filepath.ToSlash(filepath.Join("archive", "inbox",
				now.UTC().Format("2006-01-02"), "sync-conflicts", losingName))
		}
		out = append(out, res)
	}
	return out, nil
}
