//go:build windows

package browser

import (
	"os/exec"
	"syscall"
)

// detachedProcess is DETACHED_PROCESS, which syscall doesn't name: the
// process gets no console, so it can't write to the app's.
const detachedProcess = 0x00000008

// detach gives cmd no standard streams, which exec opens on the null
// device, no console and a process group of its own, so the console's
// ctrl+c never reaches it.
func detach(cmd *exec.Cmd) {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
		HideWindow:    true,
	}
}
