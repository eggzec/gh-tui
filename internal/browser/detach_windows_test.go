//go:build windows

package browser

import (
	"os/exec"
	"syscall"
)

// detached reports whether cmd starts with no console, in a process
// group of its own.
func detached(cmd *exec.Cmd) bool {
	const want = syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess
	return cmd.SysProcAttr != nil && cmd.SysProcAttr.CreationFlags&want == want
}
