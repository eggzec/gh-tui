package extedit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestWords(t *testing.T) {
	tests := []struct {
		name   string
		config string
		vars   map[string]string
		want   []string
	}{
		{name: "config first", config: "hx", vars: map[string]string{"VISUAL": "vim", "EDITOR": "nano"}, want: []string{"hx"}},
		{name: "then VISUAL", vars: map[string]string{"VISUAL": "vim", "EDITOR": "nano"}, want: []string{"vim"}},
		{name: "then EDITOR", vars: map[string]string{"EDITOR": "nano"}, want: []string{"nano"}},
		{name: "blank ones are skipped", config: "  ", vars: map[string]string{"VISUAL": "\t", "EDITOR": "nano"}, want: []string{"nano"}},
		{name: "words", config: "  code  --wait --new-window ", want: []string{"code", "--wait", "--new-window"}},
		{name: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Words(tt.config, env(tt.vars)); !slices.Equal(got, tt.want) {
				t.Errorf("Words() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestArgs(t *testing.T) {
	const f = "/tmp/gh-tui-1/main.go"
	tests := []struct {
		words []string
		want  []string
	}{
		{[]string{"vim"}, []string{"+12", f}},
		{[]string{"/usr/bin/nvim", "-u", "NONE"}, []string{"-u", "NONE", "+12", f}},
		{[]string{"vi"}, []string{"+12", f}},
		{[]string{"nano"}, []string{"+12", f}},
		{[]string{"emacsclient", "-t"}, []string{"-t", "+12", f}},
		{[]string{"emacs", "-nw"}, []string{"-nw", "+12", f}},
		{[]string{"micro"}, []string{"+12", f}},
		{[]string{"hx"}, []string{"+12", f}},
		{[]string{"helix"}, []string{"+12", f}},
		{[]string{"kak"}, []string{"+12", f}},
		{[]string{`C:\Program Files\Vim\GVIM.EXE`}, []string{"+12", f}},
		{[]string{"code", "--wait"}, []string{"--wait", "-g", f + ":12"}},
		{[]string{"codium", "-w"}, []string{"-w", "-g", f + ":12"}},
		{[]string{"subl", "-w"}, []string{"-w", f}},
		{[]string{"ed"}, []string{f}},
	}
	for _, tt := range tests {
		if got := Args(tt.words, f, 12); !slices.Equal(got, tt.want) {
			t.Errorf("Args(%q) = %q, want %q", tt.words, got, tt.want)
		}
	}
	if got := Args([]string{"vim"}, f, 0); !slices.Equal(got, []string{"+1", f}) {
		t.Errorf("line 0 gives %q, want +1", got)
	}
}

// The files to edit that a killed program left behind go once they are a
// day old, and nothing else in the directory does.
func TestSweep(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	day := now.Add(-StaleAfter - time.Minute)
	mk := func(name string, files []string, subdir bool, at time.Time) string {
		t.Helper()
		d := filepath.Join(dir, name)
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			p := filepath.Join(d, f)
			if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(p, at, at); err != nil {
				t.Fatal(err)
			}
		}
		if subdir {
			if err := os.Mkdir(filepath.Join(d, "sub"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(d, at, at); err != nil {
			t.Fatal(err)
		}
		return d
	}
	stale := mk(TempPrefix+"123", []string{"main.go"}, false, day)
	keep := make([]string, 0, 10)
	keep = append(keep,
		mk(TempPrefix+"456", []string{"main.go"}, false, now),
		mk(TempPrefix+"789", []string{"a", "b"}, false, day),
		mk(TempPrefix+"790", nil, true, day),
		mk(TempPrefix+"791", nil, false, day),
		mk(TempPrefix+"x12", []string{"main.go"}, false, day),
		mk("gh-tui-123", []string{"main.go"}, false, day),
		mk("other", []string{"main.go"}, false, day),
	)
	// A link to a directory is not one.
	target := mk("target", []string{"main.go"}, false, day)
	link := filepath.Join(dir, TempPrefix+"999")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	keep = append(keep, target, link)
	// A file in a fresh directory may still be open, even if it is old.
	fresh := mk(TempPrefix+"555", []string{"main.go"}, false, day)
	if err := os.Chtimes(filepath.Join(fresh, "main.go"), now, now); err != nil {
		t.Fatal(err)
	}
	keep = append(keep, fresh)

	Sweep(dir, now)
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Errorf("%s is still there", stale)
	}
	for _, k := range keep {
		if _, err := os.Lstat(k); err != nil {
			t.Errorf("%s is gone: %v", k, err)
		}
	}
}

// fakeExec returns an Exec that runs edit on the file the editor would
// open and then ends with err.
func fakeExec(args *[]string, mode *os.FileMode, edit func(file string), err error) Exec {
	return func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd {
		return func() tea.Msg {
			*args = c.Args
			file := c.Args[len(c.Args)-1]
			if fi, serr := os.Stat(file); serr == nil {
				*mode = fi.Mode().Perm()
			}
			if edit != nil {
				edit(file)
			}
			return done(err)
		}
	}
}

type result struct {
	text string
	err  error
}

func TestEdit(t *testing.T) {
	dir := t.TempDir()
	var args []string
	var mode os.FileMode
	run := fakeExec(&args, &mode, func(file string) {
		if err := os.WriteFile(file, []byte("edited"), 0o600); err != nil {
			t.Error(err)
		}
	}, nil)
	cmd := Edit(run, []string{"vim"}, dir, "c.md", "text", 3, func(s string, err error) tea.Msg { return result{s, err} })
	got, _ := cmd().(result)
	if got.err != nil || got.text != "edited" {
		t.Errorf("got %+v, want the edited text", got)
	}
	if len(args) != 3 || args[1] != "+3" || filepath.Base(args[2]) != "c.md" {
		t.Errorf("ran %q", args)
	}
	if mode != 0o600 {
		t.Errorf("the file's mode is %v, want 0600", mode)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("left %d files behind", len(left))
	}
}

func TestEditFails(t *testing.T) {
	dir := t.TempDir()
	var args []string
	var mode os.FileMode
	boom := errors.New("exit status 1")
	cmd := Edit(fakeExec(&args, &mode, nil, boom), []string{"vim"}, dir, "c.md", "text", 1,
		func(s string, err error) tea.Msg { return result{s, err} })
	got, _ := cmd().(result)
	if !errors.Is(got.err, boom) || got.text != "" {
		t.Errorf("got %+v, want the editor's error and no text", got)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("left %d files behind", len(left))
	}
}
