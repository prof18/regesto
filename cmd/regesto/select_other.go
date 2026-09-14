//go:build !darwin && !linux

package main

import "os"

// makeRaw is unsupported elsewhere; the picker reports errNotATerminal and
// init falls back to non-interactive behavior.
func makeRaw(f *os.File) (func(), error) {
	return nil, errNotATerminal
}

// terminalWidth has no portable query here; 0 disables line clipping.
func terminalWidth(f *os.File) int {
	return 0
}
