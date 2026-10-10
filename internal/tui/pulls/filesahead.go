package pulls

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The files of a pull request are read ahead, as prefetch.pulls.files says:
// the first page, once the modal has rested on a tab other than Files, so
// that the Files tab shows it at once when it is first shown. It is the
// modal's own read, of the pull request that is open and of no row of the
// list, and it reads at most that page: the next ones are read as the diff
// scrolls. The head names the page, so that a push the detail shows drops
// the read of the old head.

// newFilesAhead returns the read ahead of the first page of the files of
// pull request number of repo, which ctx bounds, as p says, or as the
// defaults do without it. Slots, if not nil, are shared with the other
// reads ahead.
func newFilesAhead(ctx context.Context, svc Service, repo core.RepoRef, number int, p *config.PrefetchLayers, slots *ui.Slots) *ui.Ahead[string] {
	query := func(head string) pulls.FilesQuery {
		return pulls.FilesQuery{Repo: repo, Number: number, Head: head}
	}
	a := ui.NewAhead("pull_files", func(ctx context.Context, head string) error {
		return readFirstFiles(ctx, svc, query(head))
	}, func(head string) bool {
		return svc.CurrentFiles(query(head))
	})
	a.Share(slots)
	a.Configure(config.Resolved{})
	if p != nil {
		a.Configure(filesAheadSettings(*p))
	}
	a.Reset(ctx)
	a.Rest()
	return a
}

// filesAheadSettings returns what prefetch.pulls.files resolves to, which
// reads one page for the one pull request, so it has no window.
func filesAheadSettings(p config.PrefetchLayers) config.Resolved {
	r := ui.Resolve(p, "pulls", "files")
	r.Window = config.Window{}
	return r
}

// readFirstFiles reads the page of q into the cache that the Files tab
// reads from. A page that an earlier session kept is read again, as the
// tab reads it, so that what it finds is of GitHub's answer.
func readFirstFiles(ctx context.Context, svc Service, q pulls.FilesQuery) error {
	p, err := svc.Files(ctx, q)
	if err == nil && p.Stale {
		q.Again = true
		_, err = svc.Files(ctx, q)
	}
	return err
}

// readFilesAhead tells the read ahead of the files which head to read, if
// any, which it does once the modal has rested for the rest the settings
// give. It reads none while the Files tab shows or has read the files of
// this head, nor before the detail says what the head is, and drops one it
// has not sent when the tab changes, the head moves or the modal closes.
// A read in flight when the user opens the Files tab goes on, for the tab
// to join. Call it whenever the tab, the head or the modal may have changed.
//
// The rest starts when the head to read changes, which is when the modal
// leaves the Files tab or the detail shows a new head. Keys that move
// within a tab leave the same window and so don't start it again: unlike a
// list's cursor, the user is not moving between items.
func (m *detailModal) readFilesAhead() tea.Cmd {
	if m.ahead == nil {
		return nil
	}
	head := m.detail.HeadSHA
	if head == "" {
		m.ahead.Unkeep()
		return m.ahead.Window(nil, 0)
	}
	m.ahead.Keep(head)
	if m.closed || !m.loaded || m.onFiles() || m.files != nil && m.files.head == head {
		return m.ahead.Window(nil, 0)
	}
	return m.ahead.Window(func(int) (string, bool) { return head, true }, 0)
}

// updateAhead takes what concerns the read ahead of the files: its rest
// ending, a change of settings, and GitHub answering again after it
// limited the reads; then it reads the files ahead if it is time to.
func (m *detailModal) updateAhead(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case ui.AheadMsg:
		cmd = m.ahead.Rested(msg)
	case ui.SettingsMsg:
		m.ahead.Configure(filesAheadSettings(msg.Config.Prefetch))
		// Turned on again, it waits for the rest as at the start.
		m.ahead.Rest()
	case ui.OnlineMsg:
		// A rate limit is the token's, and has lifted unless one holds.
		if !msg.Limited {
			m.ahead.Resume()
		}
	}
	return tea.Batch(cmd, m.readFilesAhead())
}
