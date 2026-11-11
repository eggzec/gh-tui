//go:build unix

package download

import (
	"os/exec"
	"testing"
)

func assertDetached(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Error("curl doesn't start in a session of its own")
	}
}
