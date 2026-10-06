//go:build !unix && !windows

package browser

import "os/exec"

// detached reports true: the platform has nothing to detach with but
// the streams, which the tests check themselves.
func detached(*exec.Cmd) bool { return true }
