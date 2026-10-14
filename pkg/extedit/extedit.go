// Package extedit opens text in the user's editor and reads it back: the
// editor command's words from the config, $VISUAL or $EDITOR, a file of the
// user's own for the text, the line to open at, and the sweep of files that
// a killed program left behind.
package extedit

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TempPrefix starts the name of the directory of each file to edit, and
// StaleAfter is how old one is when an editor it opened can't still be
// open, as after the program was killed while it was.
const (
	TempPrefix = "gh-tui-edit-"
	StaleAfter = 24 * time.Hour
)

// Exec runs a command with the terminal to itself, as [tea.ExecProcess]
// does, and returns the command that does so.
type Exec func(*exec.Cmd, tea.ExecCallback) tea.Cmd

// Words returns the words of the editor command: cmd, else $VISUAL, else
// $EDITOR as getenv reads them, or none.
//
// The command is split into words at white space, as git splits
// core.editor when it holds no shell characters, and run without a shell.
// So "code --wait" works, and no name of the content can ever be read as
// shell code; an editor whose path holds a space needs a wrapper script or
// a link.
func Words(cmd string, getenv func(string) string) []string {
	for _, c := range []string{cmd, getenv("VISUAL"), getenv("EDITOR")} {
		if words := strings.Fields(c); len(words) > 0 {
			return words
		}
	}
	return nil
}

// Edit returns a command that writes text to a file named name, readable by
// the user alone, in a directory of its own under dir (the system's for
// ""), opens it in the editor words at line, suspends the program until the
// editor exits through run, and removes the file. done gets the file's
// content once the editor exited well, or the error: of the editor, or of
// writing or reading the file. words must not be empty.
//
// A graphical editor needs the argument that makes it wait until the file
// is closed, since the file is removed once the command exits.
func Edit(run Exec, words []string, dir, name, text string, line int, done func(edited string, err error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		Sweep(dir, time.Now())
		file, remove, err := WriteTemp(dir, name, text)
		if err != nil {
			return done("", err)
		}
		c := exec.Command(words[0], Args(words, file, line)...)
		return run(c, func(err error) tea.Msg {
			defer remove()
			if err != nil {
				return done("", err)
			}
			b, rerr := os.ReadFile(file)
			if rerr != nil {
				return done("", fmt.Errorf("read the edited file: %w", rerr))
			}
			return done(string(b), nil)
		})()
	}
}

// Args returns the arguments of the editor command words for file, opened
// at line where the editor takes one: vi and its kin, nano, emacs, micro,
// helix and kakoune take +N before the file, and VS Code -g file:N. Other
// editors open the file at its start.
func Args(words []string, file string, line int) []string {
	args := slices.Clone(words[1:])
	n := strconv.Itoa(max(line, 1))
	switch lineFlag(words[0]) {
	case "+":
		return append(args, "+"+n, file)
	case "-g":
		return append(args, "-g", file+":"+n)
	}
	return append(args, file)
}

// lineFlag returns how the editor named by cmd, a name or a path, takes
// the line to open at: "+", "-g", or "" if it isn't known to take one.
func lineFlag(cmd string) string {
	name := strings.TrimSuffix(strings.ToLower(BaseName(cmd)), ".exe")
	switch name {
	case "vi", "vim", "nvim", "gvim", "mvim", "view", "vimx", "nvi", "elvis",
		"nano", "emacs", "emacsclient", "micro", "hx", "helix", "kak":
		return "+"
	case "code", "code-insiders", "codium":
		return "-g"
	}
	return ""
}

// WriteTemp writes text to a new file named name, readable by the user
// alone, in a directory of its own under dir, or the system's for "". It
// returns the file's path and a function that removes both.
func WriteTemp(dir, name, text string) (file string, remove func(), err error) {
	d, err := os.MkdirTemp(dir, TempPrefix+"*")
	if err != nil {
		return "", nil, fmt.Errorf("write the file to edit: %w", err)
	}
	remove = func() { _ = os.RemoveAll(d) }
	file = filepath.Join(d, name)
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		remove()
		return "", nil, fmt.Errorf("write the file to edit: %w", err)
	}
	return file, remove, nil
}

// BaseName returns the last element of p, a path with slashes or, as on
// Windows, backslashes.
func BaseName(p string) string {
	return path.Base(strings.ReplaceAll(p, `\`, "/"))
}

// Sweep removes the directories of files to edit that were left in dir, or
// the system's for "", more than [StaleAfter] before now, as when the
// program was killed with an editor open. It removes only what [WriteTemp]
// makes: a directory of the user's, named [TempPrefix] and digits, that
// holds one regular file, both older than [StaleAfter].
func Sweep(dir string, now time.Time) {
	if dir == "" {
		dir = os.TempDir()
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	old := func(fi os.FileInfo) bool { return now.Sub(fi.ModTime()) >= StaleAfter }
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), TempPrefix)
		if !ok || rest == "" || strings.Trim(rest, "0123456789") != "" || !e.IsDir() {
			continue
		}
		d := filepath.Join(dir, e.Name())
		fi, err := os.Lstat(d)
		if err != nil || !fi.IsDir() || !ownedByUser(fi) || !old(fi) {
			continue
		}
		files, err := os.ReadDir(d)
		if err != nil || len(files) != 1 || !files[0].Type().IsRegular() {
			continue
		}
		if ffi, err := files[0].Info(); err != nil || !ownedByUser(ffi) || !old(ffi) {
			continue
		}
		_ = os.RemoveAll(d)
	}
}
