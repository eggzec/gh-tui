//go:build unix

package browser

import (
	"os/exec"
	"syscall"
)

// detach gives cmd no standard streams, which exec opens on the null
// device, and a session of its own, so it has no controlling terminal to
// open and gets none of the terminal's signals.
func detach(cmd *exec.Cmd) {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
