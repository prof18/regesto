//go:build linux

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// makeRaw is the Linux twin of select_darwin.go: same cbreak recipe, different
// ioctl constants (see that file for why each flag is cleared).
func makeRaw(f *os.File) (func(), error) {
	var t syscall.Termios
	fd := f.Fd()
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(&t)), 0, 0, 0); errno != 0 {
		return nil, errno
	}
	saved := t
	t.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Iflag &^= syscall.IXON | syscall.ICRNL
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TCSETS, uintptr(unsafe.Pointer(&t)), 0, 0, 0); errno != 0 {
		return nil, errno
	}
	return func() {
		s := saved
		syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TCSETS, uintptr(unsafe.Pointer(&s)), 0, 0, 0)
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
