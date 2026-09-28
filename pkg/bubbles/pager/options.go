package pager

import (
	"os"
	"os/exec"

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
	// editorCmd is the editor set with WithEditor. getenv reads the
	// environment for the others, exec runs the editor, and tempDir is
	// where the file it opens goes; tests fake them.
	editorCmd string
	getenv    func(string) string
	exec      func(*exec.Cmd, tea.ExecCallback) tea.Cmd
	tempDir   string
}

// DefaultTabWidth is the number of columns between tab stops by default.
const DefaultTabWidth = 4

// DefaultHighlightLimit is the size in bytes above which content is shown
// as plain text by default. Highlighting runs in the background, but it
// takes seconds for a few megabytes.
const DefaultHighlightLimit = 1 << 20

func defaultSettings() settings {
	return settings{
		keys:           DefaultKeyMap(),
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

// WithKeyMap sets the key bindings.
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
