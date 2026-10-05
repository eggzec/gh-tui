package pager

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Notes on the editor, shown in place of the name until the next key.
const (
	noteNoEditor  = "No editor: set editor in the config, $VISUAL or $EDITOR"
	noteEditorErr = "Editor: "
	noteCopy      = "Opened a copy; changes aren't kept."
)

// copyNoted reports whether a pager said this session that the editor
// opened a copy, which it says once.
var copyNoted atomic.Bool

// tempPrefix starts the name of the directory of each file to edit, and
// staleAfter is how old one is when an editor it opened can't still be
// open, as after the program was killed while it was.
const (
	tempPrefix = "gh-tui-edit-"
	staleAfter = 24 * time.Hour
)

// editedMsg reports that the editor the pager with ID id opened its
// content in is done, and how it went.
type editedMsg struct {
	id  int64
	err error
}

// editor returns the words of the editor command: the one set with
// WithEditor, else $VISUAL, else $EDITOR, or none.
//
// The command is split into words at white space, as git splits
// core.editor when it holds no shell characters, and run without a shell.
// So "code --wait" works, and no name of the content can ever be read as
// shell code; an editor whose path holds a space needs a wrapper script or
// a link.
func (m Model) editor() []string {
	for _, cmd := range []string{m.editorCmd, m.getenv("VISUAL"), m.getenv("EDITOR")} {
		if words := strings.Fields(cmd); len(words) > 0 {
			return words
		}
	}
	return nil
}

// edit opens the content in the editor, at the line at the top of the
// window, or at its first line for rendered content, and suspends the
// program until the editor exits. The editor gets the content as it was
// given, before it was cleaned or decoded, and whatever filter hides, in
// a file of its own that is removed afterwards.
func (m *Model) edit() tea.Cmd {
	if m.state != stateReady {
		return nil
	}
	words := m.editor()
	if words == nil {
		m.flash = noteNoEditor
		return nil
	}
	id, run, dir := m.id, m.exec, m.tempDir
	name, lang, text, line := m.name, m.lang, m.raw, m.topLine()+1
	if m.render != nil {
		// A rendered line has no line of its own in the source.
		line = 1
	}
	return func() tea.Msg {
		sweepTemp(dir, time.Now())
		file, remove, err := writeTemp(dir, tempName(name, lang), text)
		if err != nil {
			return editedMsg{id: id, err: err}
		}
		c := exec.Command(words[0], editorArgs(words, file, line)...)
		return run(c, func(err error) tea.Msg {
			remove()
			return editedMsg{id: id, err: err}
		})()
	}
}

// edited shows what went wrong with the editor, if anything did, or else,
// once a session, that it opened a copy.
func (m *Model) edited(err error) {
	if err == nil {
		if copyNoted.CompareAndSwap(false, true) {
			m.info(noteCopy)
		}
		return
	}
	msg, _, _ := strings.Cut(err.Error(), "\n")
	m.flash = noteEditorErr + termtext.OneLine(msg)
}

// editorArgs returns the arguments of the editor command words for file,
// opened at line where the editor takes one: vi and its kin, nano, emacs,
// micro, helix and kakoune take +N before the file, and VS Code -g
// file:N. Other editors open the file at its start.
func editorArgs(words []string, file string, line int) []string {
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
	name := strings.TrimSuffix(strings.ToLower(baseName(cmd)), ".exe")
	switch name {
	case "vi", "vim", "nvim", "gvim", "mvim", "view", "vimx", "nvi", "elvis",
		"nano", "emacs", "emacsclient", "micro", "hx", "helix", "kak":
		return "+"
	case "code", "code-insiders", "codium":
		return "-g"
	}
	return ""
}

// tempName returns the name of the file the editor opens for content
// named name, highlighted as lang if that is set: the base of the name,
// so the editor can tell the file type, with the extension of lang where
// the name doesn't have it, such as ".diff" for a patch. A name that
// can't be a file's is "content".
func tempName(name, lang string) string {
	base := baseName(name)
	if base == "." || base == "/" || base == ".." || len(base) > 200 ||
		strings.ContainsFunc(base, func(r rune) bool { return unicode.IsControl(r) || termtext.Control(r) }) {
		base = "content"
	}
	if lang == "" {
		return base
	}
	if l := syntax.Lexer(lang); l != nil {
		for _, glob := range l.Config().Filenames {
			if ext, ok := strings.CutPrefix(glob, "*"); ok && strings.HasPrefix(ext, ".") && !strings.ContainsAny(ext, "*?[") {
				if !strings.HasSuffix(base, ext) {
					base += ext
				}
				break
			}
		}
	}
	return base
}

// writeTemp writes text to a new file named name, readable by the user
// alone, in a directory of its own under dir, or the system's for "". It
// returns the file's path and a function that removes both.
func writeTemp(dir, name, text string) (file string, remove func(), err error) {
	d, err := os.MkdirTemp(dir, tempPrefix+"*")
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

// baseName returns the last element of p, a path with slashes or, as on
// Windows, backslashes.
func baseName(p string) string {
	return path.Base(strings.ReplaceAll(p, `\`, "/"))
}

// sweepTemp removes the directories of files to edit that were left in
// dir, or the system's for "", more than staleAfter before now, as when
// the program was killed with an editor open. It removes only what
// writeTemp makes: a directory of the user's, named tempPrefix and
// digits, that holds one regular file, both older than staleAfter.
func sweepTemp(dir string, now time.Time) {
	if dir == "" {
		dir = os.TempDir()
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	old := func(fi os.FileInfo) bool { return now.Sub(fi.ModTime()) >= staleAfter }
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), tempPrefix)
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
