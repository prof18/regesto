package harvest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/prof18/regesto/internal/config"
)

// Blobs are the last-seen copy of each captured file, kept in .state/ so a
// capture can be expressed as a diff instead of a whole file.
//
// .state/ is excluded from both git and Syncthing because it is per-machine
// (PLAN 0.a), which makes it exactly the right home: the baseline costs local
// disk and nothing else, while the inbox — which *is* synced and committed —
// carries only what changed.

func sourceBlobRelativePath(cfg *config.Config, agent, sourceID, key string) string {
	return filepath.Join(".state", cfg.Machine, agent, "sources", sourceID, "last",
		strings.ReplaceAll(key, "/", "__"))
}

func readSourceBlob(kbFS *os.Root, cfg *config.Config, agent, sourceID, key string) ([]byte, error) {
	return readRootFile(kbFS, sourceBlobRelativePath(cfg, agent, sourceID, key))
}

func writeSourceBlob(kbFS *os.Root, cfg *config.Config, agent, sourceID, key string, data []byte) error {
	return writeRootAtomic(kbFS, sourceBlobRelativePath(cfg, agent, sourceID, key), data, 0o644)
}

// unifiedDiff renders old→new as a unified diff. It reports false when a diff
// is unavailable or not worth it, in which case the caller captures the whole
// file — being unable to shrink a capture must never mean losing it.
func unifiedDiff(old, new []byte, label string) ([]byte, bool) {
	if bytes.Equal(old, new) {
		return nil, false
	}
	dir, err := os.MkdirTemp("", "regesto-diff-")
	if err != nil {
		return nil, false
	}
	defer os.RemoveAll(dir)

	oldPath := filepath.Join(dir, "previous")
	newPath := filepath.Join(dir, "current")
	if os.WriteFile(oldPath, old, 0o600) != nil || os.WriteFile(newPath, new, 0o600) != nil {
		return nil, false
	}

	// `diff` exits 1 when files differ, which is the expected case; only a
	// higher status means it actually failed.
	cmd := exec.Command("diff", "-u",
		"--label", label+" (previously harvested)",
		"--label", label+" (now)",
		oldPath, newPath)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() > 1 {
			return nil, false
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	// A diff bigger than the file it describes is no saving, and the whole
	// file is easier for normalisation to read.
	if len(out) >= len(new) {
		return nil, false
	}

	header := fmt.Sprintf("# regesto harvest: changes to %s\n"+
		"# Unified diff against the previously harvested copy. Added lines (+) are\n"+
		"# what to normalise; the full file is in the agent's own store.\n\n", label)
	return append([]byte(header), out...), true
}
