package pager

import (
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Option configures a pager in [New].
type Option func(*settings)

type settings struct {
	width, height  int
	keys           KeyMap
	styles         Styles
	tabWidth       int
	lineNumbers    bool
	wrap           bool
	highlightLimit int
	errorText      func(error) (text, hint string)
	// numbersNote and chopNote explain the line numbers and the chopping
	// of rendered content, or are empty.
	numbersNote, chopNote string
	// editorCmd is the editor set with WithEditor. getenv reads the
	// environment for the others, exec runs the editor, and tempDir is
	// where the file it opens goes; tests fake them.
	editorCmd string
	// resizeRest is how long rendered content waits at a new width before
	// it renders again, or 0 to render at once.
	resizeRest time.Duration
	getenv     func(string) string
	exec       func(*exec.Cmd, tea.ExecCallback) tea.Cmd
	tempDir    string
}

// DefaultTabWidth is the number of columns between tab stops by default.
const DefaultTabWidth = 4

// DefaultHighlightLimit is the size in bytes above which content is shown
// as plain text by default. Highlighting runs in the background, but it
// takes seconds for a few megabytes.
const DefaultHighlightLimit = 1 << 20

func defaultSettings() settings {
	return settings{
		styles:         DefaultStyles(true),
		tabWidth:       DefaultTabWidth,
		lineNumbers:    true,
		highlightLimit: DefaultHighlightLimit,
		getenv:         os.Getenv,
		exec:           tea.ExecProcess,
	}
}

// WithSize sets the width and height, including the status line.
func WithSize(width, height int) Option {
	return func(s *settings) {
		s.width, s.height = width, height
	}
}

// WithKeyMap sets the key bindings. Without it no key is bound, so a
// pager shows its content and nothing else.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) {
		s.keys = k
	}
}

// WithStyles sets the styles.
func WithStyles(st Styles) Option {
	return func(s *settings) {
		s.styles = st
	}
}

// WithTabWidth sets the number of columns between tab stops. It applies to
// content set afterwards.
func WithTabWidth(n int) Option {
	return func(s *settings) {
		s.tabWidth = max(n, 1)
	}
}

// WithLineNumbers sets whether the line numbers are shown. They are by
// default.
func WithLineNumbers(show bool) Option {
	return func(s *settings) {
		s.lineNumbers = show
	}
}

// WithWrap sets whether long lines are soft-wrapped instead of scrolled
// sideways. They are scrolled by default.
func WithWrap(wrap bool) Option {
	return func(s *settings) {
		s.wrap = wrap
	}
}

// WithHighlightLimit sets the size in bytes above which content is shown
// as plain text. Zero or less turns highlighting off.
func WithHighlightLimit(bytes int) Option {
	return func(s *settings) {
		s.highlightLimit = bytes
	}
}

// WithErrorText sets how content that failed to load reads. say returns
// the words for err and a hint, such as "o to open on GitHub", or "" for
// none, styled as one. An empty text shows no error. By default the pager
// says "Couldn't load:" and the first line of the error.
func WithErrorText(say func(error) (text, hint string)) Option {
	return func(s *settings) {
		s.errorText = say
	}
}

// WithResizeRest sets how long rendered content waits, after the pager
// is resized, before it renders again at the new width, so that a
// resize that goes on, such as a window being dragged, renders it once
// it stops rather than at every width. The parent starts the wait by
// calling [Model.Settle] after each resize. It shows what it has, cut to
// the new width, in the meantime. By default, or for 0, it renders at
// once.
func WithResizeRest(d time.Duration) Option {
	return func(s *settings) {
		s.resizeRest = d
	}
}

// WithEditor sets the command of the editor that the key bound to Edit
// opens the content in, such as "vim" or "code --wait": a program and its
// arguments, split at white space and run without a shell. When it is
// empty, as by default, the pager takes $VISUAL, and else $EDITOR. A
// graphical editor needs the argument that makes it wait until the file is
// closed, since the file is removed once the command exits.
func WithEditor(cmd string) Option {
	return func(s *settings) {
		s.editorCmd = cmd
	}
}

// WithRenderedNotes sets what the pager says of the line numbers and of
// chopping or wrapping long lines while it shows rendered content, as
// [Model.SetRendered] gives it, since there the lines are those rendered
// rather than those of the source, and the render already fit them to the
// width. numbers is the whole note that turning the numbers on gets, in
// place of the usual one, with the point first, such as "Rows
// numbered"; chop is a short phrase, such as "code only", shown in
// brackets after the usual note. Both are kept short, since the status
// line has little room, and an empty one changes nothing.
func WithRenderedNotes(numbers, chop string) Option {
	return func(s *settings) {
		s.numbersNote, s.chopNote = numbers, chop
	}
}
