// Package modals owns the modals that are open over the screen: the trail of
// them, which one is on top, whether another may open over it, what closing
// one does to the others, their size and whether each is maximized.
//
// A request returns a [Change] that says what it did, and the root acts on
// it. The manager hides a modal that another covers, discards one it drops,
// gives the new top its theme and size, and tells a modal that is back with a
// [ui.ReopenedMsg].
//
// It never names a modal type, shows a toast, logs, routes keys, draws the
// frame, or reads the config. Only the root imports it.
package modals
