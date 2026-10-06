//go:build windows

package browser

import (
	"os/exec"
	"syscall"
)

// detached reports whether cmd starts with no console, in a process
// group of its own, and with its window shown.
func detached(cmd *exec.Cmd) bool {
	const want = syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess
	return cmd.SysProcAttr != nil && cmd.SysProcAttr.CreationFlags&want == want && !cmd.SysProcAttr.HideWindow
}
