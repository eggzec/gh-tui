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
	// modal is first drawn and whenever the terminal is resized.
	SetSize(width, height int)
	SetTheme(t Theme)
	Keyed
}

// Tabbed is a Modal with tabs, such as the filters of a list, which the
// app shows at the right end of the top edge of the frame.
type Tabbed interface {
	// Tabs returns the names of the tabs, and the index of the one shown,
	// or -1 for none.
	Tabs() (names []string, active int)
}

// Linked is a Modal about something that has a page on the web, such as a
// pull request, whose title in the top edge of the frame links to it.
type Linked interface {
	// Link returns the address of the page, or "" while it isn't known.
	Link() string
}

// OpenModalMsg asks the app to open Modal over the screen, in place of the
// modal that is open.
type OpenModalMsg struct {
	Modal Modal
}

// OpenModal returns a command that opens m. The opener starts whatever m
// needs to load itself once it is open, with
// tea.Sequence(ui.OpenModal(m), m.load()): the app passes messages to a
// modal only while it is open, so a load batched with the opening could
// finish first and be lost.
func OpenModal(m Modal) tea.Cmd {
	return func() tea.Msg { return OpenModalMsg{Modal: m} }
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
