package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// Modal is a popup over the screen, such as a file preview or the history.
// The app draws it in a frame over the middle of the screen, one at a time:
// a modal that is opened over another is ranked against it by its type, and a
// modal that needs more views, such as a list and a detail, shows them
// inside its own frame. While it is open it takes every key, except ctrl+c,
// which still quits; every other message reaches it as it reaches the
// sections.
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

// Saver is a Modal or a section that holds text the user wrote and hasn't
// sent, such as a comment in a composer. The app calls Save when it quits,
// by ctrl+c or the q command, so that quitting loses nothing. Save writes what it holds now, and may block on the disk; a
// failure is logged and the app quits all the same.
type Saver interface {
	Save() error
}

// Hider is a Modal that works in the background while it is open, such as
// following a run in progress, which it stops once another modal covers
// it: the app calls Hide then. It gets a [ReopenedMsg] when it is on top
// again, and starts again.
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
	// CommandReferences shows the issues and pull requests linked to what
	// the modal shows; the modal is [Referencing].
	CommandReferences = "references"
	// CommandNextFile and CommandPrevFile go to the next and the previous
	// document or file the modal shows: the modal is [Stepping], or shows a
	// diff.
	CommandNextFile = "n"
	CommandPrevFile = "p"
)

// Hosted is a Modal that shows steps whose keys are contexts of their own,
// such as the links of a pull request, and says which context of keys it is
// itself, so that what depends on the modal, such as ui.maximized and the
// words that name it, follow it whatever step it shows.
type Hosted interface {
	// KeyContext is the name of the modal's own context of keys.
	KeyContext() string
}

// HelpHost is a Modal that shows the help of its keys inside itself, over
// what it has open, instead of the app's help over the modal.
type HelpHost interface {
	// OpenHelp shows the help of layers, titled title, and reports whether
	// the modal took it: when it didn't, the app opens its own.
	OpenHelp(layers []keyhelp.Layer, title string) (cmd tea.Cmd, ok bool)
}

// Stepping is a Modal that shows several documents, one at a time, which the
// next and previous commands step through.
type Stepping interface {
	// Step shows the document delta places on, stopping at the ends.
	Step(delta int) tea.Cmd
}

// Referencing is a Modal that can show the issues and pull requests linked
// to what it shows, such as the modal of a pull request, which the
// references command asks for with the same result as its key.
type Referencing interface {
	// ShowReferences shows them, and reports whether the modal has them to
	// show, which it doesn't where nothing reads them.
	ShowReferences() (cmd tea.Cmd, ok bool)
}

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

// OpenModalMsg asks the app to open Modal over the screen. The app ranks it
// by its type against the open modal: a higher one opens over it, an equal
// one replaces it, and a lower one is refused. If Over is set, the open is
// dropped unless Over is still the open modal.
type OpenModalMsg struct {
	Modal Modal
	Over  Modal
	// Load, if set, runs only once Modal is open: a refused open reads
	// nothing.
	Load tea.Cmd
}

// OpenModalLoading returns a command that opens m as OpenModalOver does, and
// then runs load, only if m ended up open. The app passes messages to a modal
// only while it is open, so the load starts after the open.
func OpenModalLoading(m, over Modal, load tea.Cmd) tea.Cmd {
	return func() tea.Msg { return OpenModalMsg{Modal: m, Over: over, Load: load} }
}

// OpenModal returns a command that opens m. The opener starts whatever m
// needs to load itself once it is open, with
// tea.Sequence(ui.OpenModal(m), m.load()): the app passes messages to a
// modal only while it is open, so a load batched with the opening could
// finish first and be lost.
func OpenModal(m Modal) tea.Cmd {
	return func() tea.Msg { return OpenModalMsg{Modal: m} }
}

// OpenModalOver returns a command that opens m as OpenModal does, but only
// while over is still the open modal that asked for it, so that an open that
// finished after the user moved on is dropped.
func OpenModalOver(m, over Modal) tea.Cmd {
	return func() tea.Msg { return OpenModalMsg{Modal: m, Over: over} }
}

// Discarder is a Modal that the app drops without it being open, such as
// one that closed under another or that the cap of held modals evicts.
// Discard stops it for good: its reads end, and the reads it held back go on.
type Discarder interface {
	Discard()
}

// ReopenedMsg tells Modal that it is on top again, after the modal that
// covered it closed, such as a file preview opened from it. A modal that
// stopped what it did while hidden, such as polls, starts it again.
type ReopenedMsg struct {
	Modal Modal
}

// CloseModalMsg asks the app to close Modal. It does nothing unless Modal is
// the open modal.
type CloseModalMsg struct {
	Modal Modal
}

// CloseModal returns a command that closes m.
func CloseModal(m Modal) tea.Cmd {
	return func() tea.Msg { return CloseModalMsg{Modal: m} }
}

// LeaveModalMsg asks the app to leave Modal: to return to the modal under
// it, or to close it if there is none. It does nothing unless Modal is the
// open modal.
type LeaveModalMsg struct {
	Modal Modal
}

// LeaveModal returns a command that leaves m.
func LeaveModal(m Modal) tea.Cmd {
	return func() tea.Msg { return LeaveModalMsg{Modal: m} }
}
