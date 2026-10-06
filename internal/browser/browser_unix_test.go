//go:build unix

package browser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestStartWritesNothingToTheApp starts a real browser, a script that
// writes to its stdout and stderr, from a copy of this test binary whose
// streams are captured, and checks that none of it gets there.
func TestStartWritesNothingToTheApp(t *testing.T) {
	if dir := os.Getenv("BROWSER_TEST_CHILD"); dir != "" {
		openFromChild(dir)
		return
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho NOISE-OUT\necho NOISE-ERR >&2\necho \"$1\" > " + filepath.Join(dir, "ran") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "browser"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartWritesNothingToTheApp$") //nolint:gosec // The test binary itself.
	cmd.Env = append(os.Environ(), "BROWSER_TEST_CHILD="+dir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("child: %v\n%s", err, out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("NOISE")) {
		t.Errorf("the browser wrote to the app's streams:\n%s", out.String())
	}
	ran, err := os.ReadFile(filepath.Join(dir, "ran"))
	if err != nil || string(bytes.TrimSpace(ran)) != "https://github.com" {
		t.Errorf("the browser didn't run with the page: %q, %v", ran, err)
	}
}

// openFromChild opens a page with the script in dir, with a grace long
// enough for the script to end, so its output, if any, is written
// before the child exits.
func openFromChild(dir string) {
	l := New(WithEnv(func(k string) string {
		if k == "BROWSER" {
			return filepath.Join(dir, "browser")
		}
		return ""
	}), WithGHConfig(func() string { return "" }), WithGrace(time.Minute))
	if _, err := l.Open("https://github.com"); err != nil {
		os.Exit(2)
	}
}
