//go:build !unix && !windows

package download

import "os/exec"

// kill ends cmd.
func kill(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// detach gives cmd no output streams. The platform has no sessions to start
// it in.
func detach(cmd *exec.Cmd) {
	cmd.Stdout, cmd.Stderr = nil, nil
}
