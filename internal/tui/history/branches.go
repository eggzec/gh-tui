package history

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// maxBranchPages bounds the pages of branches read, of a hundred each, so
// that a repository with thousands of branches doesn't read them all.
const maxBranchPages = 10

// branchPageAt is how close to the last branch loaded the cursor gets
// before the next page is read. It pages the list; it isn't a read ahead.
const branchPageAt = 10

// branches is the branch pane: the branches of the repository, the default
// one first.
type branches struct {
	feed feed.Model[core.Branch]
	// wanted reports that the filter waits for the last page of branches,
	// which its branch field chooses from, to open.
	wanted bool
	// defaultBranch is listed first, and the others are compared with it.
	defaultBranch string
	// compares holds how far each branch compared is from the default.
	compares map[string]core.Compare
	// seq counts the moves of the cursor, so that only the last rest
	// compares.
	seq int
	// seen names the branch the cursor was last found on.
	seen string
}

// branchRestMsg reports that the cursor rested on a branch.
type branchRestMsg struct {
	id  int64
	seq int
}

// compareMsg carries how far branch is from the default branch.
type compareMsg struct {
	id     int64
	branch string
	cmp    core.Compare
	err    error
}

func (b *branches) init(defaultBranch string) {
	b.defaultBranch = defaultBranch
	b.compares = map[string]core.Compare{}
}

// branchFetch reads the pages of branches for the feed. The default branch
// leads the first page, as it is the one most often looked at, and the pages
// stop at maxBranchPages.
func (m *Modal) branchFetch() feed.Fetch[core.Branch] {
	svc, repo, def := m.svc, m.repo, m.branches.defaultBranch
	query := func(cursor string) historysvc.BranchesQuery {
		return historysvc.BranchesQuery{Repo: repo, Cursor: cursor}
	}
	read := func(ctx context.Context, q historysvc.BranchesQuery, again bool) (core.Page[core.Branch], error) {
		q.Again = again
		return svc.Branches(ctx, q)
	}
	pages := ui.FeedPages("history.branches", query, read)
	var mu sync.Mutex
	number := map[string]int{"": 1}
	return func(ctx context.Context, cursor string) ([]core.Branch, string, error) {
		items, next, err := pages(ctx, cursor)
		mu.Lock()
		n := number[cursor]
		if next != "" {
			number[next] = n + 1
		}
		mu.Unlock()
		if n >= maxBranchPages {
			next = ""
		}
		if err != nil {
			return items, next, err
		}
		i := slices.IndexFunc(items, func(b core.Branch) bool { return b.Name == def })
		switch {
		case cursor != "":
			// The first page lists the default branch, so it isn't listed twice.
			if i >= 0 {
				items = slices.Delete(slices.Clone(items), i, i+1)
			}
		case i > 0:
			items = slices.Clone(items)
			d := items[i]
			items = slices.Insert(slices.Delete(items, i, i+1), 0, d)
		case i < 0 && def != "":
			// GitHub may list the default branch on a later page; it leads
			// the list all the same, and only its name is shown.
			items = append([]core.Branch{{Name: def}}, items...)
		}
		return items, next, nil
	}
}

// newBranchFeed returns the feed of the branches.
func (m *Modal) newBranchFeed() feed.Model[core.Branch] {
	return feed.New(m.branchFetch(), func(br core.Branch, selected bool, w int) string {
		return m.branchRow(br, selected, w)
	},
		feed.WithContext(m.ctx),
		feed.WithKey(func(br core.Branch) string { return br.Name }),
		feed.WithPrefetch(branchPageAt),
		feed.WithKeyMap(m.keys.Branches),
		feed.WithEmptyText(ui.None("branches")),
		feed.WithErrorText(ui.ErrorText("load the branches", m.repo.String(), *m.opts.voice)),
	)
}

// updateBranches passes a message to the feed, then reacts to what it
// changed: a cursor on another branch, and the pages the filter waits for.
func (m *Modal) updateBranches(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.branches.feed, cmd = m.branches.feed.Update(msg)
	return tea.Batch(cmd, m.branchSeen(), m.openWanted())
}

// branchSeen starts the comparisons of the branch under the cursor when it
// is not the one last seen there, however the cursor got to it.
func (m *Modal) branchSeen() tea.Cmd {
	b := &m.branches
	br, ok := b.feed.Selected()
	if !ok || br.Name == b.seen {
		return nil
	}
	b.seen = br.Name
	return m.branchMoved()
}

// pressBranches handles a key in the branch pane.
func (m *Modal) pressBranches(msg tea.KeyPressMsg) tea.Cmd {
	b := &m.branches
	switch {
	case keymap.Matches(msg, m.keys.Select):
		br, ok := b.feed.Selected()
		if !ok {
			return nil
		}
		return m.showBranch(br.Name)
	case keymap.Matches(msg, m.keys.Filter):
		return m.openFilter()
	}
	before := b.feed.Index()
	cmd := m.updateBranches(msg)
	if b.feed.Index() != before {
		// The user moved on, so the filter no longer waits for the pages.
		b.wanted = false
	}
	return cmd
}

// branchMoved starts the delay after which the branch under the cursor is
// compared with the default branch, and the branches around it.
func (m *Modal) branchMoved() tea.Cmd {
	return tea.Batch(m.branchRest(), m.compareAround())
}

// branchRest starts the delay after which the branch under the cursor is
// compared with the default branch, unless it was.
func (m *Modal) branchRest() tea.Cmd {
	b := &m.branches
	b.seq++
	br, ok := b.feed.Selected()
	if !ok || !m.comparable(br.Name) {
		return nil
	}
	if _, ok := b.compares[br.Name]; ok {
		return nil
	}
	if c, ok := m.svc.CachedCompare(m.repo, b.defaultBranch, br.Name); ok {
		b.compares[br.Name] = c
		return nil
	}
	msg := branchRestMsg{id: m.id, seq: b.seq}
	return tea.Tick(m.opts.prefetch.branches.Rest, func(time.Time) tea.Msg { return msg })
}

// branchPair names a comparison of head with base.
type branchPair struct {
	base, head string
}

// compareAround compares, once the cursor rests, the branches in a window
// around it with the default branch, as the prefetch settings say, so
// that they show at once. The branch under the cursor is compared by
// itself, whatever they say.
func (m *Modal) compareAround() tea.Cmd {
	b := &m.branches
	cursor := b.feed.Index()
	at := func(j int) (branchPair, bool) {
		br, ok := b.feed.LoadedItem(j)
		if j == cursor || !ok || !m.comparable(br.Name) {
			return branchPair{}, false
		}
		return branchPair{b.defaultBranch, br.Name}, true
	}
	// Until the branches are listed, there is no window.
	if !b.feed.Settled() {
		at = nil
	}
	return m.compares.Window(at, cursor)
}

// readCompare compares the branches of k ahead of their use, for
// m.compares.
func (m *Modal) readCompare(ctx context.Context, k branchPair) error {
	_, err := m.svc.Compare(ctx, m.repo, k.base, k.head)
	return err
}

// cachedCompare reports whether the comparison of k is in memory.
func (m *Modal) cachedCompare(k branchPair) bool {
	_, ok := m.svc.CachedCompare(m.repo, k.base, k.head)
	return ok
}

// comparable reports whether name can be compared with the default branch.
func (m *Modal) comparable(name string) bool {
	return m.branches.defaultBranch != "" && name != m.branches.defaultBranch
}

// compare reads how far the branch the cursor rested on is from the
// default branch, unless the cursor moved on.
func (m *Modal) compare(msg branchRestMsg) tea.Cmd {
	b := &m.branches
	br, ok := b.feed.Selected()
	if msg.seq != b.seq || !ok || !m.comparable(br.Name) {
		return nil
	}
	if _, ok := b.compares[br.Name]; ok {
		return nil
	}
	svc, ctx, repo, id, base := m.svc, m.ctx, m.repo, m.id, b.defaultBranch
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "history.compare")
		c, err := svc.Compare(ctx, repo, base, br.Name)
		end(err, "span", "tui", "repo", repo.String(), "head", br.Name)
		return compareMsg{id: id, branch: br.Name, cmp: c, err: err}
	}
}

// receiveCompare keeps a comparison. A failed one shows nothing: the
// branch is still there to look at.
func (m *Modal) receiveCompare(msg compareMsg) {
	if msg.err == nil {
		m.branches.compares[msg.branch] = msg.cmp
	}
}

// isBase reports whether the files are shown at name, or at a commit
// chosen on it.
func (m *Modal) isBase(name string) bool {
	if m.base.Ref == "" {
		return name != "" && name == m.defaultBranch
	}
	return m.base.Ref == name || m.base.Branch == name
}

// branchLines renders the branch pane's body, h lines of w cells.
func (m *Modal) branchLines(w, h int) []string {
	return padLines(strings.Split(m.branches.feed.View(), "\n"), w, h)
}

// branchRow renders one branch in w cells: its name, how far it is from the
// default branch once compared, and whether the files are shown at it.
func (m *Modal) branchRow(br core.Branch, _ bool, w int) string {
	var tags []string
	tagsW := 0
	if c, ok := m.branches.compares[br.Name]; ok && br.Name != m.branches.defaultBranch {
		t := m.st.ic.Up + strconv.Itoa(c.AheadBy) + m.st.ic.Down + strconv.Itoa(c.BehindBy)
		tags, tagsW = append(tags, m.st.subtle.Render(t)), ansi.StringWidth(t)
	}
	if m.isBase(br.Name) {
		if tagsW > 0 {
			tagsW++
		}
		tags, tagsW = append(tags, m.st.baseMark), tagsW+1
	}
	style := m.st.text
	switch {
	case br.Name == m.graph.shown():
		style = m.st.accent
	case br.Name == m.branches.defaultBranch:
		style = m.st.strong
	}
	nameRoom := w
	if tagsW > 0 && w-tagsW-1 >= min(ansi.StringWidth(br.Name), 8) {
		nameRoom = w - tagsW - 1
	} else {
		tags, tagsW = nil, 0
	}
	name := termtext.Truncate(ui.OneLine(br.Name), nameRoom, m.st.ic.Ellipsis)
	line := style.Render(name)
	if tagsW == 0 {
		return line
	}
	pad := w - ansi.StringWidth(name) - tagsW
	return line + strings.Repeat(" ", max(pad, 1)) + strings.Join(tags, " ")
}
