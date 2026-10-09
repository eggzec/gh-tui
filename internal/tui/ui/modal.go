package ui

import (
	tea "charm.land/bubbletea/v2"
)

// Modal is a popup over the screen, such as a file preview or the history.
// The app draws it in a frame over the middle of the screen. Modals never
// stack: opening one replaces the one that is open, so a modal that needs
// more views, such as a list and a detail, shows them inside its own frame. While it is open it takes every key, except ctrl+c, which still
// quits; every other message reaches it as it reaches the sections.
//
// A Modal is implemented by a pointer, since the app finds the one to close
// by comparing it. It closes itself by returning [CloseModal].
type Modal interface {
	// Title is shown in the top edge of the frame.
	Title() string
	Update(msg tea.Msg) tea.Cmd
	// View fits the size of the last SetSize exactly.
	View() string
	// SetSize sets the size inside the frame. The app calls it before the
	// modal is first drawn and whenever its room changes, as when the
	// terminal is resized.
	SetSize(width, height int)
	SetTheme(t Theme)
	Keyed
	Commanded
}

// Settler is a Modal that waits out a resize before it does work that is
// costly at every width, such as rendering markdown again.
type Settler interface {
	// Settle returns the command that ends the wait the last SetSize
	// began, or nil when nothing waits. The app calls it after each
	// update that resized the modal: the terminal's resize, or a change
	// of what shares the screen, such as the command line opening.
	Settle() tea.Cmd
}

// Tabbed is a Modal with tabs, such as the filters of a list, which the
// app shows at the right end of the top edge of the frame.
type Tabbed interface {
	// Tabs returns the names of the tabs, and the index of the one shown,
	// or -1 for none.
	Tabs() (names []string, active int)
}

// Hider is a Modal that works in the background while it is open, such as
// following a run in progress, which it stops once another modal replaces
// it: the app calls Hide then. A modal that opened the one in
// its place, such as a preview, gets a [ReopenedMsg] when it is back, and
// starts again; one that another replaced for good stays paused.
type Hider interface {
	Hide()
}

// Commanded is what a Modal says of the command line, which the command
// key opens over every modal unless it types the key as text, as a query
// or a question does.
type Commanded interface {
	// Commands names the commands, beyond those that act on the app alone,
	// that the modal takes: those that act on what it shows, such as the
	// raw command on a file. The app refuses the others over it, naming
	// the modal, and completes only the ones that run.
	Commands() []string
}

// The commands that a modal may name in [Commanded.Commands].
const (
	// CommandCopy copies the url, ref, sha or path of what the modal shows;
	// the modal is a [Selector].
	CommandCopy = "copy"
	// CommandRaw shows the file the modal shows as its source or rendered;
	// the modal is [Sourced].
	CommandRaw = "raw"
)

// Sourced is a view that shows a file rendered, such as markdown, or as
// its source, which the raw command switches between.
type Sourced interface {
	// Raw reports whether the view shows the source, and ok whether it
	// shows a file it can render at all.
	Raw() (raw, ok bool)
	// SetRaw shows the source if raw is set, and the file rendered
	// otherwise.
	SetRaw(raw bool) tea.Cmd
}

// Linked is a Modal about something that has a page on the web, such as a
// pull request, whose title in the top edge of the frame links to it.
type Linked interface {
	// Link returns the address of the page, or "" while it isn't known.
	Link() string
}

// OpenModalMsg asks the app to open Modal over the screen, in place of the
// modal that is open. If Back is that modal, the app keeps it, hidden, so
// that the back key returns to it.
type OpenModalMsg struct {
	Modal Modal
	Back  Modal
}

// OpenModal returns a command that opens m. The opener starts whatever m
// needs to load itself once it is open, with
// tea.Sequence(ui.OpenModal(m), m.load()): the app passes messages to a
// modal only while it is open, so a load batched with the opening could
// finish first and be lost.
func OpenModal(m Modal) tea.Cmd {
	return func() tea.Msg { return OpenModalMsg{Modal: m} }
}

// OpenModalOver returns a command that opens m in place of back, the modal
// that is open and asked for it, such as a pull request picked in the
// references of another. The back key returns to back, which stays as it
// was; if back is no longer the open modal, m opens as OpenModal opens it.
func OpenModalOver(m, back Modal) tea.Cmd {
	return func() tea.Msg { return OpenModalMsg{Modal: m, Back: back} }
}

// Discarder is a Modal that the app drops without it being open, such as
// one that the back key leaves, or that the end of a chain of modals
// leaves behind. Discard stops it for good: its reads end, and the reads
// it held back go on.
type Discarder interface {
	Discard()
}

// ReopenedMsg tells Modal that it is open again, after a modal it opened
// in its place closed, such as a file preview opened from it. A modal that
// stopped what it did while hidden, such as polls, starts it again.
type ReopenedMsg struct {
	Modal Modal
}

// Reopen returns a command that opens m again, in place of the modal that
// is open, and then tells it with a [ReopenedMsg].
func Reopen(m Modal) tea.Cmd {
	return tea.Sequence(OpenModal(m), func() tea.Msg { return ReopenedMsg{Modal: m} })
}

// CloseModalMsg asks the app to close Modal. It does nothing once another
// modal has replaced it.
type CloseModalMsg struct {
	Modal Modal
}

// CloseModal returns a command that closes m.
func CloseModal(m Modal) tea.Cmd {
	return func() tea.Msg { return CloseModalMsg{Modal: m} }
}
