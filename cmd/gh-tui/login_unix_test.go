//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A symbolic link named like a token's directory is not followed: it and
// the old directory it points at both survive a sweep.
func TestSweepTokensKeepsSymlinks(t *testing.T) {
	cfg := loginDisk(t)
	old := keptToken(t, cfg, "old-token", testNow.Add(-2*unusedFor))
	target := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	age(t, target, testNow.Add(-2*unusedFor))
	link := filepath.Join(filepath.Dir(old), tokenKey("github.com", "linked-token"))
	if err := os.Symlink(target, link); err != nil {
		t.Skip("no symbolic links:", err)
	}
	sweepTokens(cfg, "github.com", envToken, testNow)
	waitGone(t, old)
	for _, p := range []string{link, target} {
		if !exists(p) {
			t.Errorf("%s was removed", filepath.Base(p))
		}
	}
}

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
