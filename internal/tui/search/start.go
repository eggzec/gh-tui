package search

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// startItem is a row of what the page offers before the user types: a
// recent search, or a repository.
type startItem struct {
	// header names the group the items below it are in.
	header string
	query  string
	repo   *core.Repo
}

// startList is what the page offers before the user types: the searches
// of this session, then the repositories of the Start function.
type startList struct {
	repos   []core.Repo
	loaded  bool
	loading bool
	err     error

	items []startItem
	// rows holds the index in items of each row with a cursor stop.
	rows   []int
	sel    int
	top    int
	height int
}

// startMsg carries the repositories of the Start function, for the page
// with id.
type startMsg struct {
	id    int64
	repos []core.Repo
	err   error
}

func (s *Section) loadStart() tea.Cmd {
	if s.start == nil || s.starts.loaded || s.starts.loading {
		return nil
	}
	s.starts.loading = true
	start, ctx, id := s.start, s.ctx, s.id
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "search.start")
		repos, err := start(ctx)
		end(err, "span", "tui", "items", len(repos))
		return startMsg{id: id, repos: repos, err: err}
	}
}

func (s *Section) started(msg startMsg) {
	st := &s.starts
	st.loading, st.loaded, st.err = false, msg.err == nil, msg.err
	if msg.err == nil {
		st.repos = msg.repos
	}
	st.build(s.recent)
}

// build lists recent after the searches, and the repositories after them.
func (l *startList) build(recent []string) {
	items := make([]startItem, 0, len(recent)+len(l.repos)+2)
	rows := make([]int, 0, len(recent)+len(l.repos))
	if len(recent) > 0 {
		items = append(items, startItem{header: "Recent searches"})
		for _, q := range recent {
			rows = append(rows, len(items))
			items = append(items, startItem{query: q})
		}
	}
	if len(l.repos) > 0 {
		items = append(items, startItem{header: "Your repositories"})
		for i := range l.repos {
			rows = append(rows, len(items))
			items = append(items, startItem{repo: &l.repos[i]})
		}
	}
	l.items, l.rows = items, rows
	l.sel = min(l.sel, max(len(rows)-1, 0))
	l.scroll()
}

func (l *startList) selected() (startItem, bool) {
	if l.sel >= len(l.rows) {
		return startItem{}, false
	}
	return l.items[l.rows[l.sel]], true
}

func (l *startList) move(delta int) {
	if len(l.rows) == 0 {
		return
	}
	l.sel = min(max(l.sel+delta, 0), len(l.rows)-1)
	l.scroll()
}

func (l *startList) resize(height int) {
	l.height = max(height, 0)
	l.scroll()
}

// scroll shows the row of the cursor, with the header of its group above
// its first item.
func (l *startList) scroll() {
	if l.height <= 0 || len(l.rows) == 0 {
		l.top = 0
		return
	}
	row := l.rows[l.sel]
	first := row
	if first > 0 && l.items[first-1].header != "" {
		first--
	}
	if first < l.top {
		l.top = first
	}
	if row >= l.top+l.height {
		l.top = row - l.height + 1
	}
	l.top = min(max(l.top, 0), max(len(l.items)-l.height, 0))
}
