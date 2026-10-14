package pager

import (
	"strings"
	"sync/atomic"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/extedit"
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

// editedMsg reports that the editor the pager with ID id opened its
// content in is done, and how it went.
type editedMsg struct {
	id  int64
	err error
}

// editor returns the words of the editor command: the one set with
// WithEditor, else $VISUAL, else $EDITOR, or none.
func (m Model) editor() []string {
	return extedit.Words(m.editorCmd, m.getenv)
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
	return extedit.Edit(run, words, dir, tempName(name, lang), text, line,
		func(_ string, err error) tea.Msg { return editedMsg{id: id, err: err} })
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

// tempName returns the name of the file the editor opens for content
// named name, highlighted as lang if that is set: the base of the name,
// so the editor can tell the file type, with the extension of lang where
// the name doesn't have it, such as ".diff" for a patch. A name that
// can't be a file's is "content".
func tempName(name, lang string) string {
	base := extedit.BaseName(name)
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
