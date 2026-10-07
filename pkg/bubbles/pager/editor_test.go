package pager

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// env returns a getenv that reads vars.
func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestEditorPrecedence(t *testing.T) {
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
			m := fresh(t, WithEditor(tt.config))
			m.getenv = env(tt.vars)
			if got := m.editor(); !slices.Equal(got, tt.want) {
				t.Errorf("editor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEditorArgs(t *testing.T) {
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
		if got := editorArgs(tt.words, f, 12); !slices.Equal(got, tt.want) {
			t.Errorf("editorArgs(%q) = %q, want %q", tt.words, got, tt.want)
		}
	}
	if got := editorArgs([]string{"vim"}, f, 0); !slices.Equal(got, []string{"+1", f}) {
		t.Errorf("line 0 gives %q, want +1", got)
	}
}

func TestTempName(t *testing.T) {
	tests := []struct{ name, lang, want string }{
		{"internal/tui/main.go", "", "main.go"},
		{"Makefile", "", "Makefile"},
		{"", "", "content"},
		{"..", "", "content"},
		{"a/", "", "a"},
		{`dir\notes.txt`, "", "notes.txt"},
		{"evil\x1b[2J.txt", "", "content"},
		{"rtl\u202etxt.exe", "", "content"},
		{"$(rm -rf ~); x.sh", "", "$(rm -rf ~); x.sh"},
		{"cmd/main.go", "diff", "main.go.diff"},
		{"fix.diff", "diff", "fix.diff"},
		{"main.go", "no-such-lexer", "main.go"},
	}
	for _, tt := range tests {
		if got := tempName(tt.name, tt.lang); got != tt.want {
			t.Errorf("tempName(%q, %q) = %q, want %q", tt.name, tt.lang, got, tt.want)
		}
	}
}

// ran is what a fake exec saw of the editor it ran.
type ran struct {
	args    []string
	content string
	mode    os.FileMode
	file    string
}

// fakeExec returns an exec that records what it would run and the file
// it would open, and then ends with err.
func fakeExec(tb testing.TB, got *ran, err error) func(*exec.Cmd, tea.ExecCallback) tea.Cmd {
	tb.Helper()
	return func(c *exec.Cmd, done tea.ExecCallback) tea.Cmd {
		return func() tea.Msg {
			got.args = c.Args
			got.file = c.Args[len(c.Args)-1]
			b, rerr := os.ReadFile(got.file)
			if rerr != nil {
				tb.Errorf("read the file to edit: %v", rerr)
			}
			if fi, serr := os.Stat(got.file); serr == nil {
				got.mode = fi.Mode().Perm()
			}
			got.content = string(b)
			return done(err)
		}
	}
}

// editorPager returns a focused pager showing text, named name, that runs a
// fake editor in dir.
func editorPager(tb testing.TB, name, text, dir string, got *ran, err error) Model {
	tb.Helper()
	m := open(tb, name, text, WithSize(40, 6), WithEditor("vim"))
	m.exec, m.tempDir = fakeExec(tb, got, err), dir
	return m
}

// The editor gets the content as it was given, whatever the pager shows
// of it, in a file of the user's own that is gone once it exits.
func TestEdit(t *testing.T) {
	text := "Gr\xfc\xdfe \x1b[31mred\x1b[m\tend\n" + numbered(50)
	dir := t.TempDir()
	copyNoted.Store(false)
	var got ran
	m := editorPager(t, "docs/notes.txt", text, dir, &got, nil)
	m, _ = enterAll(t, m, "&", "line", "enter")
	m, _ = keys(t, m, "j", "j", "j", "j", "j", "j", "j", "j")
	m, cmd := m.Update(press("v"))
	if cmd == nil {
		t.Fatal("v ran nothing")
	}
	m, _ = m.Update(cmd())
	if want := []string{"vim", "+10", filepath.Join(filepath.Dir(got.file), "notes.txt")}; !slices.Equal(got.args, want) {
		t.Errorf("ran %q, want %q", got.args, want)
	}
	if !strings.HasPrefix(got.file, dir) {
		t.Errorf("the file %s isn't in %s", got.file, dir)
	}
	if got.content != text {
		t.Errorf("the editor got %q, want the content as given", got.content)
	}
	if got.mode != 0o600 {
		t.Errorf("the file's mode is %v, want 0600", got.mode)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("left %d files behind", len(left))
	}
	if m.flash != noteCopy || !m.flashInfo || m.Filter() != "line" {
		t.Errorf("after the editor: note %q, filter %q; want %q and the filter kept", m.flash, m.Filter(), noteCopy)
	}
	// The note on the copy is said once a session.
	m, cmd = m.Update(press("v"))
	m, _ = m.Update(cmd())
	if m.flash != "" {
		t.Errorf("after the editor again: note %q, want none", m.flash)
	}
}

// The files to edit that a killed program left behind go once they are a
// day old, and nothing else in the directory does.
func TestSweepTemp(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	day := now.Add(-staleAfter - time.Minute)
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
	stale := mk(tempPrefix+"123", []string{"main.go"}, false, day)
	keep := make([]string, 0, 10)
	keep = append(keep,
		mk(tempPrefix+"456", []string{"main.go"}, false, now),
		mk(tempPrefix+"789", []string{"a", "b"}, false, day),
		mk(tempPrefix+"790", nil, true, day),
		mk(tempPrefix+"791", nil, false, day),
		mk(tempPrefix+"x12", []string{"main.go"}, false, day),
		mk("gh-tui-123", []string{"main.go"}, false, day),
		mk("other", []string{"main.go"}, false, day),
	)
	// A link to a directory is not one.
	target := mk("target", []string{"main.go"}, false, day)
	link := filepath.Join(dir, tempPrefix+"999")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	keep = append(keep, target, link)
	// A file in a fresh directory may still be open, even if it is old.
	fresh := mk(tempPrefix+"555", []string{"main.go"}, false, day)
	if err := os.Chtimes(filepath.Join(fresh, "main.go"), now, now); err != nil {
		t.Fatal(err)
	}
	keep = append(keep, fresh)

	sweepTemp(dir, now)
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Errorf("%s is still there", stale)
	}
	for _, k := range keep {
		if _, err := os.Lstat(k); err != nil {
			t.Errorf("%s is gone: %v", k, err)
		}
	}
}

// A patch opens as a diff, for the editor to tell.
func TestEditSyntax(t *testing.T) {
	var got ran
	m := fresh(t, append(defaults(t), WithSize(40, 6), WithEditor("vim"))...)
	m.Focus()
	m.exec, m.tempDir = fakeExec(t, &got, nil), t.TempDir()
	_ = m.SetContentSyntax("cmd/main.go", "diff", "@@ -1 +1 @@\n-a\n+b\n")
	_, cmd := m.Update(press("v"))
	_ = cmd()
	if filepath.Base(got.file) != "main.go.diff" {
		t.Errorf("opened %s, want main.go.diff", got.file)
	}
}

func TestEditFails(t *testing.T) {
	var got ran
	m := editorPager(t, "a.txt", "text\n", t.TempDir(), &got, errors.New("exit status 1\nmore"))
	m, cmd := m.Update(press("v"))
	m, _ = m.Update(cmd())
	if m.flash != "Editor: exit status 1" || m.flashInfo {
		t.Errorf("note %q (info %v), want the error", m.flash, m.flashInfo)
	}
	if !strings.Contains(lastLine(plain(m)), "Editor: exit status 1") {
		t.Errorf("status %q doesn't say the editor failed", lastLine(plain(m)))
	}
	// A message of another pager is not this one's.
	m, _ = keys(t, m, "j")
	m, _ = m.Update(editedMsg{id: m.ID() + 1, err: errors.New("other")})
	if m.flash != "" {
		t.Errorf("took another pager's message: %q", m.flash)
	}
}

func TestEditWithoutEditor(t *testing.T) {
	m := open(t, "a.txt", "text\n", WithSize(80, 4))
	m.getenv = env(nil)
	m, cmd := m.Update(press("v"))
	if cmd != nil || m.flash != noteNoEditor {
		t.Errorf("cmd %v, note %q; want none and %q", cmd != nil, m.flash, noteNoEditor)
	}
	if !strings.Contains(lastLine(plain(m)), "No editor") {
		t.Errorf("status %q doesn't say there is no editor", lastLine(plain(m)))
	}
}

// v doesn't fire while the pager waits for text, an option or the key
// after a count, nor when it shows no content.
func TestEditCaptured(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		set    func(*Model)
		prompt string
		note   string
	}{
		{name: "search prompt", keys: []string{"/"}, prompt: "v"},
		{name: "filter prompt", keys: []string{"&"}, prompt: "v"},
		{name: "option", keys: []string{"-"}, note: noteNoOption + "v"},
		{name: "loading", set: func(m *Model) { _ = m.SetLoading("a.txt") }},
		{name: "message", set: func(m *Model) { m.SetMessage("a.bin", "Binary") }},
		{name: "binary", set: func(m *Model) { _ = m.SetContent("a.bin", "a\x00b") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ran
			m := editorPager(t, "a.txt", "text\n", t.TempDir(), &got, nil)
			if tt.set != nil {
				tt.set(&m)
			}
			m, _ = keys(t, m, tt.keys...)
			if m.FullHelp()[3][6].Enabled() {
				t.Error("help shows v enabled")
			}
			m, cmd := m.Update(press("v"))
			if cmd != nil {
				if msg := cmd(); msg != nil {
					if _, ok := msg.(editedMsg); ok {
						t.Fatal("v opened the editor")
					}
				}
			}
			if tt.prompt != "" && m.prompt.Value() != tt.prompt {
				t.Errorf("prompt %q, want %q", m.prompt.Value(), tt.prompt)
			}
			if m.flash != tt.note {
				t.Errorf("note %q, want %q", m.flash, tt.note)
			}
		})
	}
}
