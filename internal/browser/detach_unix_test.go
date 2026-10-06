//go:build unix

package browser

import "os/exec"

// detached reports whether cmd starts in a session of its own.
func detached(cmd *exec.Cmd) bool {
	return cmd.SysProcAttr != nil && cmd.SysProcAttr.Setsid
}
