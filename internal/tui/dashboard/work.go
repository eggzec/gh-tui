package dashboard

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// workRow is a row of a list of the work pane: one of its items, or what
// stands in for them.
type workRow struct {
	hit *core.SearchHit
	// note is shown when a list is empty.
	note string

	// ref is where an item is, such as "eggzec/cli#12".
	ref string
}

// workTab is a list of the work pane, with its own cursor and scroll. Its
// items come first in rows, then a note if it has one.
type workTab struct {
	count int
	// refused is set when GitHub refused the list's search, which the
	// pane says in place of its rows.
	refused bool
	rows    []workRow
	items   int
	// next is the cursor of the page after the last item, empty when all
	// are listed. loading is set while that page is read, and err holds why
	// the read failed.
	next    string
	loading bool
	err     error
	sel     int
	// top is the first row on view.
	top int
	// first is how many items the first page listed, and last where the
	// items of the last page start, which a page read again replaces.
	first, last int
	// epoch counts the times the first page was listed again, so a page
	// read before that is dropped when it arrives.
	epoch int
}

// workList is the work pane: a tab for each list of work waiting on the
// viewer, one on view at a time, with a cursor over its items. An item
// takes a line, in columns: its state, where it is, its title and when it
// was updated.
type workList struct {
	tabs [len(workLists)]workTab
	cur  int
	// chosen is set once the viewer picks a tab, which new work then
	// doesn't change.
	chosen bool
	// width is the width of the pane inside its frame, and height the
	// lines under the tabs.
	width, height int
	// refW and ageW are the widths of the columns of references and ages.
	refW, ageW int
	// now is the clock that ages are measured against, and dates tell
	// them.
	now   func() time.Time
	dates ui.Dates
	// ellipsis ends a reference cut short.
	ellipsis string
}

// workIndent is the room before the text of a work item: the gutter, and
// the state and its space.
const workIndent = 4

// The lists of the work pane, the titles of their tabs in full and short,
// what each says when it is empty, and what reads it, for when GitHub
// refuses that.
var workLists = [...]struct {
	title, short, empty, action string
	list                        func(*core.Work) core.WorkList
}{
	{"Review requests", "Reviews", ui.None("review requests"), "load the pull requests that ask for your review", func(w *core.Work) core.WorkList { return w.ReviewRequested }},
	{"Your pull requests", "Mine", ui.None("open pull requests of yours"), "load your pull requests", func(w *core.Work) core.WorkList { return w.Authored }},
	{"Assigned issues", "Assigned", ui.None("open issues assigned to you"), "load the issues assigned to you", func(w *core.Work) core.WorkList { return w.Assigned }},
}

// set lists w, and keeps the cursor of each tab on the item it was on if
// it is still listed. The items of the pages after the first that a tab
// had read stay, unless the first page lists them now. Until the viewer
// picks a tab, the first that has items is on view.
func (l *workList) set(w core.Work) {
	for i, wl := range workLists {
		t := &l.tabs[i]
		var prev string
		if it, ok := t.selected(); ok {
			prev = it.Issue.URL
		}
		list := wl.list(&w)
		rows := make([]workRow, 0, len(list.Items)+1)
		seen := make(map[string]bool, len(list.Items))
		for j := range list.Items {
			rows = append(rows, workRow{hit: &list.Items[j]})
			seen[list.Items[j].Issue.URL] = true
		}
		next := list.Next
		last := len(rows)
		if !list.Refused && t.items > t.first {
			// The viewer had paged on: keep the place.
			for _, r := range t.rows[t.first:t.items] {
				if !seen[r.hit.Issue.URL] {
					rows = append(rows, workRow{hit: r.hit})
				}
			}
			next = t.next
		}
		items := len(rows)
		if !list.Refused && items == 0 {
			rows = append(rows, workRow{note: wl.empty})
		}
		*t = workTab{count: list.Count, refused: list.Refused, rows: rows, items: items, next: next, top: t.top, first: len(list.Items), last: last, epoch: t.epoch + 1}
		for j := range t.items {
			if prev != "" && rows[j].hit.Issue.URL == prev {
				t.sel = j
			}
		}
	}
	if !l.chosen {
		l.cur = 0
		for i := range l.tabs {
			if l.tabs[i].items > 0 {
				l.cur = i
				break
			}
		}
	}
	l.layout()
	for i := range l.tabs {
		l.tabs[i].scroll(l.height)
	}
}

// wantMore reports whether the page after the items is due: the cursor is
// within a page of the end.
func (t *workTab) wantMore(height int) bool {
	return t.next != "" && !t.loading && t.err == nil && t.sel >= t.items-max(height, 1)
}

// appendPage lists the items of the page that followed those of tab i, or
// with replace, in place of those of its last page, which was read again.
func (l *workList) appendPage(i int, list core.WorkList, replace bool) {
	t := &l.tabs[i]
	t.loading, t.err = false, nil
	if replace {
		t.items = min(t.last, t.items)
	} else {
		t.last = t.items
	}
	rows := make([]workRow, 0, len(t.rows)+len(list.Items))
	rows = append(rows, t.rows[:t.items]...)
	for j := range list.Items {
		rows = append(rows, workRow{hit: &list.Items[j]})
	}
	t.rows, t.items, t.next = rows, t.items+len(list.Items), list.Next
	t.sel = min(t.sel, max(t.items-1, 0))
	if list.Count > 0 {
		t.count = list.Count
	}
	l.layout()
	t.scroll(l.height)
}

// count is how many items wait on the viewer in all.
func (l *workList) count(w core.Work) int {
	return w.ReviewRequested.Count + w.Authored.Count + w.Assigned.Count
}

// current is the tab on view.
func (l *workList) current() *workTab { return &l.tabs[l.cur] }

func (l *workList) selected() (core.SearchHit, bool) {
	return l.current().selected()
}

func (t *workTab) selected() (core.SearchHit, bool) {
	if t.sel >= t.items {
		return core.SearchHit{}, false
	}
	return *t.rows[t.sel].hit, true
}

func (l *workList) move(delta int) {
	t := l.current()
	if t.items == 0 {
		return
	}
	t.sel = min(max(t.sel+delta, 0), t.items-1)
	t.scroll(l.height)
}

// switchTab shows the next tab, or a previous one for a negative delta,
// with its cursor where it was left.
func (l *workList) switchTab(delta int) {
	n := len(l.tabs)
	l.cur = ((l.cur+delta)%n + n) % n
	l.chosen = true
}

// resize sizes the pane to width by height inside its frame, the tabs
// included.
func (l *workList) resize(width, height int) {
	width, height = max(width, 0), max(height-2, 0)
	if width != l.width {
		l.width = width
		l.layout()
	}
	l.height = height
	for i := range l.tabs {
		l.tabs[i].scroll(height)
	}
}

// Headers of the columns of an item.
const (
	refHeader   = "Ref"
	titleHeader = "Title"
	ageHeader   = "Updated"
)

// layout sizes the columns of the items to the pane: the reference is as
// wide as the widest, a third of the pane at most, and the age as wide as
// the widest or its header.
func (l *workList) layout() {
	now := l.now()
	l.refW, l.ageW = 0, ansi.StringWidth(ageHeader)
	for i := range l.tabs {
		for j := range l.tabs[i].items {
			r := &l.tabs[i].rows[j]
			r.ref = r.hit.Issue.Repo.String() + "#" + strconv.Itoa(r.hit.Issue.Number)
			l.refW = max(l.refW, ansi.StringWidth(r.ref))
			l.ageW = max(l.ageW, ansi.StringWidth(l.dates.Short(r.hit.Issue.UpdatedAt, now)))
		}
	}
	l.refW = min(l.refW, l.width/3)
}

// title is the width of the title column in the pane, or 0 when there is
// no room for it.
func (l *workList) title() int {
	return max(l.width-workIndent-l.refW-1-1-l.ageW, 0)
}

// cell is ref in a column of width cells. A reference too long loses the
// end of its repository, never its number. A cut ends in tail.
func cell(ref string, width int, tail string) string {
	if ansi.StringWidth(ref) <= width {
		return ref
	}
	i := strings.LastIndexByte(ref, '#')
	num := ref[i:]
	if room := width - ansi.StringWidth(num); room > 0 {
		return termtext.Truncate(ref[:i], room, tail) + num
	}
	return termtext.Truncate(num, width, tail)
}

// header names the columns of the items.
func (l *workList) header() string {
	return strings.Repeat(" ", workIndent) + ui.Header([]ui.Column{
		{Title: refHeader, Width: l.refW},
		{Title: titleHeader, Width: l.title()},
		{Title: ageHeader, Width: l.ageW},
	}, 1, l.ellipsis)
}

// scroll shows the item under the cursor, and fills the window from the
// bottom.
func (t *workTab) scroll(height int) {
	// A line that says a page is read, or failed, takes a row of the window.
	if t.loading || t.err != nil {
		height--
	}
	t.top = t.fit(t.top, height)
}

// fit is the first row on view, for a window of height rows that has been
// scrolled to top: the one that shows the item under the cursor.
func (t *workTab) fit(top, height int) int {
	if height <= 0 || len(t.rows) == 0 {
		return top
	}
	if t.sel < t.items {
		top = min(max(top, t.sel-height+1), t.sel)
	}
	// The last rows fill the window rather than leave it half empty.
	last := len(t.rows) - 1
	return min(max(top, 0), last, max(len(t.rows)-height, 0))
}
