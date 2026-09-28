// Package ui holds what the app and its sections share: the Section
// interface, the theme, key bindings and the messages sections send to the
// app.
package ui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// Section is a top-level view of the app, shown in a framed pane of a
// screen. Sections are initialized lazily, the first time they are shown.
//
// The app sends every message to every section, since the bubbles inside
// filter messages by their ID, but key presses go to the focused one only.
type Section interface {
	Title() string
	Init() tea.Cmd
	Update(msg tea.Msg) tea.Cmd
	View() string
	SetSize(width, height int)
	SetTheme(t Theme)
	Focus()
	Blur()
	// Help lists the keys of the section, for the help line.
	Help() help.KeyMap
}

// Keyed lists its keys for help, in layers in the order it matches them,
// so that a key reaches the first enabled binding that holds it. Each
// layer holds every binding of its part, disabled ones too. A part that
// takes every key while it is open, such as a question waiting for its
// answer, is the only layer while it does.
type Keyed interface {
	KeyLayers() []keyhelp.Layer
}

// Badger is a Section that has a badge for the app's header, such as a
// count of unread notifications. An empty badge shows nothing.
type Badger interface {
	Badge() string
}

// Revisiter is a Section that has something to do when the user comes
// back to it from another screen, such as reading again what went stale
// meanwhile. Focus shows it at once; the command Revisit returns updates
// it.
type Revisiter interface {
	Revisit() tea.Cmd
}

// FileFinder is a Section that finds its files by their path, such as the
// files of the repository, which the find-file key opens from any pane of
// the repository screen.
type FileFinder interface {
	// FindFile returns the modal that finds a file, and the command that
	// loads it once it is open, or a nil modal when there is nothing to
	// find, such as before a repository is selected.
	FindFile() (Modal, tea.Cmd)
}

// Capturer is a Section that at times takes every key, such as while the
// user types into a prompt. Until Capturing reports false, the app sends
// keys straight to it, including its own keys such as quit and next pane,
// so that typing a "q" doesn't quit. Only ctrl+c still quits.
type Capturer interface {
	Capturing() bool
}
