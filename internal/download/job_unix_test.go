//go:build unix

package download

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// script returns a Runner whose "curl" is a shell script with body.
func script(t *testing.T, body string) *Runner {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "curl")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return New(WithLookPath(func(string) (string, error) { return bin, nil }))
}

func TestCancelKillsCurl(t *testing.T) {
	t.Parallel()
	r := script(t, "sleep 60")
	path := filepath.Join(t.TempDir(), "a")
	job, err := r.Start("https://x", path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".part", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	job.Cancel()
	select {
	case err := <-job.Done():
		if !errors.Is(err, ErrCancelled) {
			t.Errorf("Done = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("reaper did not return after Cancel")
	}
	if _, err := os.Stat(path + ".part"); err == nil {
		t.Error("partial file kept")
	}
}

func TestFailureReported(t *testing.T) {
	t.Parallel()
	r := script(t, "exit 22")
	path := filepath.Join(t.TempDir(), "a")
	job, err := r.Start("https://x", path, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = <-job.Done()
	if err == nil || !strings.Contains(err.Error(), "curl exit 22") {
		t.Errorf("Done = %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a file under the real name after a failure")
	}
}
