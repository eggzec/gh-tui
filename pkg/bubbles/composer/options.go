package composer

import (
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
)

// Option configures a composer in [New].
type Option func(*settings)

type settings struct {
	title         string
	placeholder   string
	width, height int
	keys          KeyMap
	styles        prompt.Styles
	drafts        Drafts
	// editorCmd is the editor set with WithEditor. getenv reads the
	// environment for the others, exec runs the editor, and tempDir is where
	// the file it opens goes; tests fake them.
	editorCmd string
	getenv    func(string) string
	exec      func(*exec.Cmd, tea.ExecCallback) tea.Cmd
	tempDir   string
}

func defaultSettings() settings {
	return settings{
		styles: prompt.DefaultStyles(true),
		getenv: os.Getenv,
		exec:   tea.ExecProcess,
	}
}

// WithTitle sets the title shown above the text.
func WithTitle(title string) Option {
	return func(s *settings) { s.title = title }
}

// WithPlaceholder sets the text shown while the composer is empty.
func WithPlaceholder(placeholder string) Option {
	return func(s *settings) { s.placeholder = placeholder }
}

// WithSize sets the width and height.
func WithSize(width, height int) Option {
	return func(s *settings) { s.width, s.height = width, height }
}

// WithKeyMap sets the key bindings, which NewKeyMap makes. Without them
// every binding is disabled, and no key acts.
func WithKeyMap(k KeyMap) Option {
	return func(s *settings) { s.keys = k }
}

// WithStyles sets the styles, those of a prompt, which the composer wears.
// The default is prompt.DefaultStyles(true).
func WithStyles(st prompt.Styles) Option {
	return func(s *settings) { s.styles = st }
}

// WithDrafts sets where drafts are kept. Without it the composer keeps none.
func WithDrafts(d Drafts) Option {
	return func(s *settings) { s.drafts = d }
}

// WithEditor sets the command of the editor that the key bound to Edit
// opens the text in, such as "vim" or "code --wait": a program and its
// arguments, split at white space and run without a shell. When it is
// empty, as by default, the composer takes $VISUAL, and else $EDITOR. A
// graphical editor needs the argument that makes it wait until the file is
// closed.
func WithEditor(cmd string) Option {
	return func(s *settings) { s.editorCmd = cmd }
}
