//go:build windows

package download

import (
	"os/exec"
	"syscall"
)

// detachedProcess is DETACHED_PROCESS, which syscall doesn't name: the
// process gets no console, so it can't write to the app's.
const detachedProcess = 0x00000008

// kill ends cmd.
func kill(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// detach gives cmd no output streams, no console and a process group of its
// own, so the console's ctrl+c never reaches it.
func detach(cmd *exec.Cmd) {
	cmd.Stdout, cmd.Stderr = nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}
