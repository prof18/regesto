//go:build !unix

package fsutil

import (
	"errors"
	"os"
)

// fileNlink has no portable implementation off Unix, so exclusive publication
// stops here rather than guessing. Only the last fallback needs it: a hard link
// or a rename still publishes normally, and this path is reached only when the
// filesystem rejects both. Failing is the safe answer — without a link count
// there is no way to tell our own reservation from a file another writer put at
// the same name, and overwriting that is the one thing publication must never
// do.
func fileNlink(f *os.File) (uint64, error) {
	return 0, errors.New("exclusive publish needs a link count, which " + f.Name() + " cannot report on this platform")
}
