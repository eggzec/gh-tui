package ui

import (
	"context"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
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
// reports a DoneMsg. Each op is a trace of its own, which logs that it was
// sent, and whether the server confirmed it or it was rolled back.
func Do(ctx context.Context, from string, op Op, what string) tea.Cmd {
	return func() tea.Msg {
		ctx := obs.WithTrace(ctx, "op")
		start := time.Now()
		slog.InfoContext(ctx, "op sent", "span", "op", "section", from, "what", what)
		err := op.Do(ctx)
		took := slog.Float64("duration_ms", obs.Millis(time.Since(start)))
		if err != nil {
			slog.WarnContext(ctx, "op rolled back", "span", "op", "section", from, "what", what, took, "err", err.Error())
		} else {
			slog.InfoContext(ctx, "op confirmed", "span", "op", "section", from, "what", what, took)
		}
		return DoneMsg{From: from, What: what, Err: err}
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

// BaseMsg sets the base of Repo, the branch or commit that its files are
// shown at. The app shows the base in the header, then passes the message
// on to the sections. A RepoMsg resets the base to the head of the default
// branch.
type BaseMsg struct {
	Repo core.RepoRef
	// Ref is a branch, or the full SHA of a commit. Empty means the head of
	// the default branch.
	Ref string
	// Label names the base in the header, such as "main @ a1b2c3d" for a
	// commit of main.
	Label string
	// Branch is the branch the base was chosen from, so that the history
	// opens on it again. It is empty for the head of the default branch.
	Branch string
}

// ResetBase returns a command that resets the base of repo to the head of
// its default branch.
func ResetBase(repo core.RepoRef) tea.Cmd {
	return func() tea.Msg { return BaseMsg{Repo: repo} }
}

// OpenPullMsg asks for the pull request Number of Repo to be opened, such as
// when it is picked in the search. Repo need not be the selected repository.
type OpenPullMsg struct {
	Repo   core.RepoRef
	Number int
}

// OpenIssueMsg asks for the issue Number of Repo to be opened, such as when
// it is picked in the search. Repo need not be the selected repository.
type OpenIssueMsg struct {
	Repo   core.RepoRef
	Number int
}

// OpenFileMsg asks for the file at Path of Repo to be previewed over the
// screen on view, such as when a code search found it. SHA is its blob.
// Repo need not be the selected repository, and the files section keeps
// showing its own.
type OpenFileMsg struct {
	Repo core.RepoRef
	Path string
	SHA  string
	// Find, if set, is searched for in the preview, which opens on its
	// first match, such as the text a code search matched.
	Find string
}

// ShowMsg asks the app to switch to the section with Title.
type ShowMsg struct {
	Title string
}

// BackMsg asks the app to go back to the screen before the one on view,
// such as when the user leaves the search.
type BackMsg struct{}

// OpenMsg asks the app to open URL in the browser.
type OpenMsg struct {
	URL string
}

// Open returns a command that asks the app to open url in the browser.
func Open(url string) tea.Cmd {
	return func() tea.Msg { return OpenMsg{URL: url} }
}
