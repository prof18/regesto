//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package notify

import "os/exec"

// Unsupported systems retain exec.CommandContext's safe direct-process kill.
func configureProcessGroup(cmd *exec.Cmd) {}
