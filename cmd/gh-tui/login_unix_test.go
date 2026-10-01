//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A pipe where the login is kept is never opened, since reading it would
// wait for ever.
func TestReadLoginRefusesAPipe(t *testing.T) {
	path := loginPath(loginDisk(t), "github.com", fakeToken)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skip("no pipes:", err)
	}
	if k := readLogin(path); k != (keptLogin{}) {
		t.Errorf("readLogin = %+v, want none", k)
	}
}
