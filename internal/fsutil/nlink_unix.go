//go:build unix

package fsutil

import (
	"fmt"
	"os"
	"syscall"
)

// fileNlink reports how many names still point at the open file's inode.
// Reading it from the held descriptor is what lets exclusive publication tell
// "my reservation is still linked" from "a failed rename deleted the name"
// without resolving the path again: inode numbers are recycled promptly on
// Linux, so comparing identity through the directory proves nothing.
func fileNlink(f *os.File) (uint64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("stat for %s: no syscall.Stat_t", f.Name())
	}
	return uint64(st.Nlink), nil
}
