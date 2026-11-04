package issues

import (
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// pinnedMsg is the answer to a read of the pinned issues of repo.
type pinnedMsg struct {
	repo  core.RepoRef
	items []core.Issue
	err   error
}

// pinKeys holds the numbers of the pinned issues, which the pages of the
// list leave out since the feed shows them above. The fetch of the list
// reads it from another goroutine.
type pinKeys struct {
	set atomic.Pointer[map[int]struct{}]

	mu sync.Mutex
	// left is how many pinned issues each page, by its cursor, had taken out,
	// so that the count of the list can add them back. A page read again
	// replaces its entry rather than adding to it.
	left map[string]int
}

// note records that the page at cursor had n pinned issues taken out.
func (p *pinKeys) note(cursor string, n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.left == nil {
		p.left = map[string]int{}
	}
	p.left[cursor] = n
}

// dropped is how many pinned issues the pages read so far had taken out.
func (p *pinKeys) dropped() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, v := range p.left {
		n += v
	}
	return n
}

// store replaces the numbers with those of items, and reports whether the
// set changed, by a number gained or lost.
func (p *pinKeys) store(items []core.Issue) (changed bool) {
	old := p.set.Load()
	next := make(map[int]struct{}, len(items))
	for i := range items {
		n := items[i].Number
		next[n] = struct{}{}
		if old == nil {
			changed = true
		} else if _, ok := (*old)[n]; !ok {
			changed = true
		}
	}
	if old != nil && len(*old) != len(next) {
		changed = true
	}
	p.set.Store(&next)
	return changed
}

// drop returns items without the pinned ones. It never changes items.
func (p *pinKeys) drop(items []core.Issue) []core.Issue {
	set := p.set.Load()
	if set == nil || len(*set) == 0 {
		return items
	}
	pinned := func(it core.Issue) bool {
		_, ok := (*set)[it.Number]
		return ok
	}
	if !slices.ContainsFunc(items, pinned) {
		return items
	}
	return slices.DeleteFunc(slices.Clone(items), pinned)
}

// clampPinned returns the pinned issues the feed shows.
func clampPinned(items []core.Issue) []core.Issue {
	return items[:min(len(items), feed.MaxPinned)]
}

// cachedPinned returns the pinned issues of the repository that the service
// already has, for the first paint.
func (s *Section) cachedPinned() []core.Issue {
	if !s.hasRepo || s.svc == nil {
		return nil
	}
	items, _ := s.svc.CachedPinned(s.repo)
	return clampPinned(items)
}

// readPinned reads the pinned issues of the repository. A repository that
// pins none, or one whose pins can't be read, shows no block.
func (s *Section) readPinned() tea.Cmd {
	if !s.live() || s.svc == nil {
		return nil
	}
	ctx, svc, repo := s.ctx, s.svc, s.repo
	return func() tea.Msg {
		items, err := svc.Pinned(ctx, repo)
		return pinnedMsg{repo: repo, items: items, err: err}
	}
}

// landPinned shows the pinned issues that were read, and none when the read
// failed. The list is read again when the set of pinned issues changed, since
// its pages were fetched with the old set dropped.
func (s *Section) landPinned(msg pinnedMsg) tea.Cmd {
	if !s.hasRepo || !msg.repo.Same(s.repo) {
		return nil
	}
	var items []core.Issue
	if msg.err != nil {
		slog.WarnContext(s.ctx, "read pinned issues", "span", "tui.issues", "repo", msg.repo.String(), "err", msg.err)
	} else {
		items = clampPinned(msg.items)
	}
	changed := s.pins.store(items)
	s.list.SetPinned(items)
	if changed && s.live() {
		return s.list.Reload()
	}
	return nil
}

// onPinned reports whether the cursor is on a pinned issue.
func (s *Section) onPinned() bool {
	_, ok := s.list.SelectedPinned()
	return ok
}

// renderPinGutters renders the gutter of a pinned row, as wide as the mark
// gutter: the cursor, then the pinned glyph.
func (s *Section) renderPinGutters() {
	st := s.theme.Feed(s.icons)
	cursor := termtext.Cells(st.CursorGlyph, 1)
	pin := st.Mark.Render(termtext.Cells(s.icons.Pinned, 1)) + " "
	s.pinGutters = [3]string{
		st.Cursor.Render(cursor) + pin,
		st.BlurredCursor.Render(cursor) + pin,
		" " + pin,
	}
}

// pinGutter is the gutter of a pinned row, drawn like the mark gutter.
func (s *Section) pinGutter(selected, focused bool) string {
	switch {
	case !selected:
		return s.pinGutters[2]
	case focused:
		return s.pinGutters[0]
	}
	return s.pinGutters[1]
}
