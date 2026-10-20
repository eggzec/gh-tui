package dashboard

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// workRow is a row of a list of the work pane: one of its items, or what
// stands in for them.
type workRow struct {
	hit *core.SearchHit
	// note is shown when a list is empty, or holds more than it lists.
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
	sel     int
	// top is the first row on view.
	top int
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
// it is still listed. Until the viewer picks a tab, the first that has
// items is on view.
func (l *workList) set(w core.Work) {
	for i, wl := range workLists {
		t := &l.tabs[i]
		var prev string
		if it, ok := t.selected(); ok {
			prev = it.Issue.URL
		}
		list := wl.list(&w)
		rows := make([]workRow, 0, len(list.Items)+1)
		for j := range list.Items {
			rows = append(rows, workRow{hit: &list.Items[j]})
		}
		switch more := list.Count - len(list.Items); {
		case list.Refused:
		case len(list.Items) == 0:
			rows = append(rows, workRow{note: wl.empty})
		case more > 0:
			rows = append(rows, workRow{note: "and " + strconv.Itoa(more) + " more on GitHub"})
		}
		*t = workTab{count: list.Count, refused: list.Refused, rows: rows, items: len(list.Items), top: t.top}
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
		return ownerui.Truncate(ref[:i], room, tail) + num
	}
	return ownerui.Truncate(num, width, tail)
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
	if height <= 0 || len(t.rows) == 0 {
		return
	}
	if t.sel < t.items {
		t.top = min(max(t.top, t.sel-height+1), t.sel)
	}
	// The last rows fill the window rather than leave it half empty.
	last := len(t.rows) - 1
	t.top = min(max(t.top, 0), last, max(len(t.rows)-height, 0))
}
