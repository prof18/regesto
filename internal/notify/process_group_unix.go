//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package notify

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup makes cancellation include shell-wrapper descendants.
// The notifier is deliberately isolated in its own group so killing it cannot
// affect the scheduler or the parent process.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
