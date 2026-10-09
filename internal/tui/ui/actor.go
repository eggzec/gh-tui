package ui

import tea "charm.land/bubbletea/v2"

// Actor is what takes an intent of the keys that work everywhere, such as
// quit or owner, by its action name without the context: the app offers
// the intent to the innermost thing that has the keys, a modal, before it
// acts on it itself. A key that nothing takes reaches the app's own rule
// for it.
type Actor interface {
	// Act does the action and returns its command, or reports that it
	// doesn't take it, so that the app goes on.
	Act(action string) (cmd tea.Cmd, handled bool)
}

// ActQuit is the intent of the quit key: what a modal does with it is to
// close.
const ActQuit = "quit"

// ActBack is the intent of the back key: what a modal does with it is to
// step back inside itself. A modal that has no step back takes it and does
// nothing, so that the key doesn't reach what is inside.
const ActBack = "back"

// ActDismiss is the intent of the dismiss key: what a modal does with it is
// to clear what is transient, such as a search, and when nothing is, to
// close.
const ActDismiss = "dismiss"
