//go:build darwin

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// makeRaw puts the terminal into cbreak mode and returns a restore function.
// ECHO off so keystrokes do not double-print, ICANON off so keys arrive one
// read at a time instead of line-buffered, ISIG/IEXTEN off so Ctrl-C reaches
// the picker as a byte it can act on (and the screen stays clean), IXON off so
// Ctrl-S does not freeze output, ICRNL off so Enter arrives as \r. Output
// processing stays on — the frame writes plain \n.
//
// VMIN=1/VTIME=0 makes Read block on at least one byte; without it a cbreak
// terminal can return immediately with nothing.
func makeRaw(f *os.File) (func(), error) {
	var t syscall.Termios
	fd := f.Fd()
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TIOCGETA, uintptr(unsafe.Pointer(&t)), 0, 0, 0); errno != 0 {
		return nil, errno
	}
	saved := t
	t.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Iflag &^= syscall.IXON | syscall.ICRNL
	t.Cc[16] = 1 // VMIN
	t.Cc[17] = 0 // VTIME
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TIOCSETA, uintptr(unsafe.Pointer(&t)), 0, 0, 0); errno != 0 {
		return nil, errno
	}
	return func() {
		s := saved
		syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TIOCSETA, uintptr(unsafe.Pointer(&s)), 0, 0, 0)
	}, nil
}

type winsize struct {
	Row, Col, Xpixel, Ypixel uint16
}

// terminalWidth reports the terminal's column count, or 0 when unknown.
func terminalWidth(f *os.File) int {
	var ws winsize
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws))); errno != 0 {
		return 0
	}
	return int(ws.Col)
}
