//go:build !unix && !windows

package browser

import "os/exec"

// detach gives cmd no standard streams, which exec opens on the null
// device. The platform has no sessions to start it in.
func detach(cmd *exec.Cmd) {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
}
