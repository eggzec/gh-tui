//go:build unix

package download

import (
	"os/exec"
	"syscall"
)

// kill ends cmd and everything in its session.
func kill(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// detach gives cmd no output streams, which exec opens on the null device,
// and a session of its own, so it has no controlling terminal and gets none
// of the terminal's signals.
func detach(cmd *exec.Cmd) {
	cmd.Stdout, cmd.Stderr = nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
