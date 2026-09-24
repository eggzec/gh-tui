package dashboard

import (
	"github.com/eggzec/gh-tui/internal/core"
)

// workRow is a line of the work pane: the header of a list, one of its
// items, or what stands in for them.
type workRow struct {
	// header names a list, with its count.
	header string
	count  int
	hit    *core.SearchHit
	// note is shown when a list is empty, or holds more than it lists.
	note string
}

// workList is the work pane: the three lists of work waiting on the
// viewer, one after the other, with a cursor over their items.
type workList struct {
	rows []workRow
	// items holds the index in rows of each item, in order.
	items []int
	sel   int
	top   int
	// height is how many rows show.
	height int
}

// The lists of the work pane, and what each says when it is empty.
var workLists = []struct {
	title, empty string
	list         func(*core.Work) core.WorkList
}{
	{"Review requests", "No pull request asks for your review.", func(w *core.Work) core.WorkList { return w.ReviewRequested }},
	{"Your pull requests", "You have no open pull request.", func(w *core.Work) core.WorkList { return w.Authored }},
	{"Assigned issues", "No open issue is assigned to you.", func(w *core.Work) core.WorkList { return w.Assigned }},
}

// set lists w, and keeps the cursor on the item it was on if it is still
// listed.
func (l *workList) set(w core.Work) {
	var prev string
	if it, ok := l.selected(); ok {
		prev = it.Issue.URL
	}
	rows := make([]workRow, 0, 16)
	items := make([]int, 0, 16)
	for _, wl := range workLists {
		list := wl.list(&w)
		rows = append(rows, workRow{header: wl.title, count: list.Count})
		if len(list.Items) == 0 {
			rows = append(rows, workRow{note: wl.empty})
			continue
		}
		for i := range list.Items {
			items = append(items, len(rows))
			rows = append(rows, workRow{hit: &list.Items[i]})
		}
		if more := list.Count - len(list.Items); more > 0 {
			rows = append(rows, workRow{note: "and " + itoa(more) + " more on GitHub"})
		}
	}
	l.rows, l.items = rows, items
	l.sel = 0
	for i, r := range items {
		if prev != "" && rows[r].hit.Issue.URL == prev {
			l.sel = i
		}
	}
	l.scroll()
}

// count is how many items wait on the viewer in all.
func (l *workList) count(w core.Work) int {
	return w.ReviewRequested.Count + w.Authored.Count + w.Assigned.Count
}

func (l *workList) selected() (core.SearchHit, bool) {
	if l.sel >= len(l.items) {
		return core.SearchHit{}, false
	}
	return *l.rows[l.items[l.sel]].hit, true
}

func (l *workList) move(delta int) {
	if len(l.items) == 0 {
		return
	}
	l.sel = min(max(l.sel+delta, 0), len(l.items)-1)
	l.scroll()
}

func (l *workList) resize(height int) {
	l.height = max(height, 0)
	l.scroll()
}

// scroll shows the row of the cursor, with the header of its list when
// there is room, and fills the window from the bottom.
func (l *workList) scroll() {
	if l.height <= 0 {
		return
	}
	if l.sel < len(l.items) {
		row := l.items[l.sel]
		// The header of a list comes into view with its first item.
		if row > 0 && l.rows[row-1].header != "" {
			row--
		}
		if row < l.top {
			l.top = row
		}
		if r := l.items[l.sel]; r >= l.top+l.height {
			l.top = r - l.height + 1
		}
	}
	l.top = min(max(l.top, 0), max(len(l.rows)-l.height, 0))
}
