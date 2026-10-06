//go:build windows

package browser

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// detachedProcess is DETACHED_PROCESS, which syscall doesn't name: the
// process gets no console, so it can't write to the app's.
const detachedProcess = 0x00000008

// detach gives cmd no standard streams, which exec opens on the null
// device, no console and a process group of its own, so the console's
// ctrl+c never reaches it. Its window shows as usual: a browser started
// hidden would stay hidden.
func detach(cmd *exec.Cmd) {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}
}

// shellOpen hands url to the default browser, as gh does on Windows.
func shellOpen(url string) error {
	return windows.ShellExecute(0, nil, windows.StringToUTF16Ptr(url), nil, nil, windows.SW_SHOWNORMAL)
}
