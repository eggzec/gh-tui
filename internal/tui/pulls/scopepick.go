package pulls

import (
	"context"
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The finder step that picks the changes the Files tab shows: all of them,
// those since the last visit, or those of one commit.

// commitsRead is the commits of the pull request as read, or why not.
type commitsRead struct {
	page core.Page[core.Commit]
	err  error
}

// commitsMsg carries the commits that the picker waits for to the modal. It
// names the channel the picker's list loads from, so that the commits of a
// picker that was closed are dropped.
type commitsMsg struct {
	ready chan commitsRead
	read  commitsRead
}

// sameScope reports whether a and b show the same changes.
func sameScope(a, b filesScope) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case scopeSince:
		return a.base == b.base
	case scopeCommit:
		return a.sha == b.sha
	case scopeAll:
	}
	return true
}

// openScope shows the picker over the Files tab. The commits come from the
// cache when they are there, and otherwise are read while the picker says it
// loads. Without a head to read the files at, it does nothing.
func (m *detailModal) openScope() tea.Cmd {
	if m.find != nil || !m.canReadFiles() {
		return nil
	}
	// A merge that waits for the detail would ask over the picker.
	m.merging = nil
	m.findHead = m.detail.HeadSHA
	current := filesScope{}
	if m.files != nil {
		current = m.files.scope
	}
	lead := []finder.Item{scopeItem("All changes", filesScope{}, current)}
	if s, ok := m.sinceScope(); ok {
		lead = append(lead, scopeItem("Since you looked ("+plural(s.commits, "commit")+")", s, current))
	}

	ctx, stop := context.WithCancel(m.ctx)
	ready := make(chan commitsRead, 1)
	q := pulls.CommitsQuery{Repo: m.repo, Number: m.number, Head: m.detail.HeadSHA}
	var read tea.Cmd
	if page, ok := m.svc.CachedCommits(q); ok {
		ready <- commitsRead{page: page}
	} else {
		svc := m.svc
		read = func() tea.Msg {
			page, err := svc.Commits(ctx, q)
			return commitsMsg{ready: ready, read: commitsRead{page: page, err: err}}
		}
	}
	f := finder.New(listScopes(lead, current, ready),
		finder.WithContext(ctx),
		finder.WithKeyMap(m.keys.finder),
		finder.WithStyles(m.theme.Finder(m.icons)),
		finder.WithPlaceholder("Type to find a commit"),
		finder.WithNoun("scope", "scopes"),
		finder.WithErrorText(m.findErr),
	)
	f.Focus()
	f.SetSize(m.width, m.height)
	m.find, m.stopFind, m.scopeReady = &f, stop, ready
	return tea.Batch(f.Init(), read)
}

// scopeItem is the row of the picker for scope s, marked if it is the current
// scope.
func scopeItem(label string, s, current filesScope) finder.Item {
	return finder.Item{Path: label, Detail: scopeMark(s, current, ""), Value: s}
}

// scopeMark says that s is the scope shown, before detail.
func scopeMark(s, current filesScope, detail string) string {
	if !sameScope(s, current) {
		return detail
	}
	if detail == "" {
		return "current"
	}
	return "current" + " · " + detail
}

// listScopes returns what the picker loads: the rows of lead, then the
// commits that arrive on ready, newest first.
func listScopes(lead []finder.Item, current filesScope, ready <-chan commitsRead) finder.Load {
	return func(ctx context.Context) (finder.Listing, error) {
		var r commitsRead
		select {
		case <-ctx.Done():
			return finder.Listing{}, ctx.Err()
		case r = <-ready:
		}
		if r.err != nil {
			return finder.Listing{}, r.err
		}
		now := time.Now()
		items := slices.Clone(lead)
		for i := range slices.Backward(r.page.Items) {
			c := &r.page.Items[i]
			s := filesScope{kind: scopeCommit, sha: c.SHA, subject: c.Subject}
			label := termtext.OneLine(c.SHA[:min(len(c.SHA), 7)] + " " + c.Subject)
			items = append(items, finder.Item{Path: label, Detail: scopeMark(s, current, commitByline(*c, now)), Value: s})
		}
		var note string
		if r.page.Truncated {
			note = "only the first " + strconv.Itoa(core.MaxPullCommits) + " commits"
		}
		return finder.Listing{Items: items, Note: note}, nil
	}
}

// commitByline is who wrote c and how long ago.
func commitByline(c core.Commit, now time.Time) string {
	who := c.Author.Name
	if who == "" {
		who = c.Author.Login
	}
	if c.Author.Date.IsZero() {
		return who
	}
	if who == "" {
		return ui.Ago(c.Author.Date, now)
	}
	return who + " · " + ui.Ago(c.Author.Date, now)
}

// receiveCommits gives the picker the commits it waits for, or closes it with
// a toast that says why they couldn't be read.
func (m *detailModal) receiveCommits(msg commitsMsg) tea.Cmd {
	if m.find == nil || msg.ready != m.scopeReady {
		return nil
	}
	if msg.read.err == nil {
		msg.ready <- msg.read
		return nil
	}
	m.dropFind()
	return ui.Notify(toast.Error, "Couldn't read the commits: "+termtext.OneLine(msg.read.err.Error()))
}

// chooseScope closes the picker and shows the changes of s on the Files tab.
func (m *detailModal) chooseScope(s filesScope) tea.Cmd {
	head := m.findHead
	m.dropFind()
	if m.detail.HeadSHA != head {
		return nil
	}
	return m.setFilesScope(s)
}
