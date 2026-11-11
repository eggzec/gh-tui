//go:build !unix

package download

import (
	"os/exec"
	"testing"
)

func assertDetached(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.SysProcAttr == nil {
		t.Log("platform has no detach attributes")
	}
}
