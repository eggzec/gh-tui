package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// NotifyMsg asks the app to show a toast.
type NotifyMsg struct {
	Level toast.Level
	Text  string
}

// Notify returns a command that shows a toast.
func Notify(level toast.Level, text string) tea.Cmd {
	return func() tea.Msg { return NotifyMsg{Level: level, Text: text} }
}

// Op is a change that is already shown and still has to be sent, such as an
// *optimistic.Op.
type Op interface {
	Do(ctx context.Context) error
}

// DoneMsg reports that an Op finished. Err is set when the server refused
// the change and it was rolled back. The app shows the error, then passes
// the message on so the section that sent it can re-render from the cache.
type DoneMsg struct {
	// From is the title of the section that sent the change.
	From string
	// What names the change for the error toast, such as "merge #42".
	What string
	Err  error
}

// Do returns a command that sends op for the section titled from and
// reports a DoneMsg.
func Do(ctx context.Context, from string, op Op, what string) tea.Cmd {
	return func() tea.Msg {
		return DoneMsg{From: from, What: what, Err: op.Do(ctx)}
	}
}

// SyncMsg reports that the data behind Key may have changed on the server.
type SyncMsg struct {
	Key string
	Err error
}

// RepoMsg selects the repository that repository-scoped sections show.
type RepoMsg struct {
	Repo core.RepoRef
}

// ShowMsg asks the app to switch to the section with Title.
type ShowMsg struct {
	Title string
}

// OpenMsg asks the app to open URL in the browser.
type OpenMsg struct {
	URL string
}

// Open returns a command that asks the app to open url in the browser.
func Open(url string) tea.Cmd {
	return func() tea.Msg { return OpenMsg{URL: url} }
}
