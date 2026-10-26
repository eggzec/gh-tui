package modals

import tea "charm.land/bubbletea/v2"

// Reason says why the app closes a modal that has a guard.
type Reason int

// The reasons. Dismiss is an esc run, a backspace the modal did not take (Back
// asks the guard of the modal it pops), or the eviction of the oldest modal; Quit is the app leaving.
const (
	Dismiss Reason = iota
	Quit
)

// Closing is a guard's answer; the zero value closes.
type Closing struct {
	// Keep refuses the close.
	Keep bool
	// Toast is shown by the root when the close is refused.
	Toast string
	// Cmd runs whether or not the modal closes.
	Cmd tea.Cmd
}

// Guard is asked before the app closes the modal it was added with.
type Guard func(Reason) Closing

// Close answers that the modal may close.
func Close() Closing { return Closing{} }

// Refuse answers that the modal stays, with a toast saying why.
func Refuse(toast string) Closing { return Closing{Keep: true, Toast: toast} }

// CloseAfter answers that the modal may close, and runs cmd.
func CloseAfter(cmd tea.Cmd) Closing { return Closing{Cmd: cmd} }
