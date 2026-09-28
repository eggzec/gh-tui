package dashboard

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// workRow is a row of a list of the work pane: one of its items, or what
// stands in for them.
type workRow struct {
	hit *core.SearchHit
	// note is shown when a list is empty, or holds more than it lists.
	note string

	// ref and lines are an item wrapped to the pane: ref, such as
	// "cli#12", starts the first line, and lines hold its title, the first
	// after ref. The last line leaves room for the age.
	ref   string
	lines []string
}

// workTab is a list of the work pane, with its own cursor and scroll. Its
// items come first in rows, then a note if it has one.
type workTab struct {
	count int
	rows  []workRow
	items int
	sel   int
	// top is the first row on view.
	top int
}

// workList is the work pane: a tab for each list of work waiting on the
// viewer, one on view at a time, with a cursor over its items. An item
// takes as many lines as its title needs, and the window scrolls by whole
// rows.
type workList struct {
	tabs [len(workLists)]workTab
	cur  int
	// chosen is set once the viewer picks a tab, which new work then
	// doesn't change.
	chosen bool
	// width is the width of the pane inside its frame, and height the
	// lines under the tabs.
	width, height int
	// now is the clock that ages are measured against.
	now func() time.Time
}

// workIndent is the room before the text of a work item: the gutter, and
// the state and its space.
const workIndent = 4

// The lists of the work pane, the titles of their tabs in full and short,
// and what each says when it is empty.
var workLists = [...]struct {
	title, short, empty string
	list                func(*core.Work) core.WorkList
}{
	{"Review requests", "Reviews", ui.None("review requests"), func(w *core.Work) core.WorkList { return w.ReviewRequested }},
	{"Your pull requests", "Mine", ui.None("open pull requests of yours"), func(w *core.Work) core.WorkList { return w.Authored }},
	{"Assigned issues", "Assigned", ui.None("open issues assigned to you"), func(w *core.Work) core.WorkList { return w.Assigned }},
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
		case len(list.Items) == 0:
			rows = append(rows, workRow{note: wl.empty})
		case more > 0:
			rows = append(rows, workRow{note: "and " + itoa(more) + " more on GitHub"})
		}
		*t = workTab{count: list.Count, rows: rows, items: len(list.Items), top: t.top}
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
	l.wrap()
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
	width, height = max(width, 0), max(height-1, 0)
	if width != l.width {
		l.width = width
		l.wrap()
	}
	l.height = height
	for i := range l.tabs {
		l.tabs[i].scroll(height)
	}
}

// wrap breaks the items into lines of the pane's width.
func (l *workList) wrap() {
	for i := range l.tabs {
		for j := range l.tabs[i].items {
			r := &l.tabs[i].rows[j]
			age := ansi.StringWidth(ui.Ago(r.hit.Issue.UpdatedAt, l.now()))
			r.ref, r.lines = wrapWork(r.hit.Issue, l.width-workIndent, age)
		}
	}
}

// wrapWork breaks the reference and title of is into lines of width
// cells. The reference starts the first line, cut to half of it at most,
// and lines holds the title, the first after the reference. The last line
// leaves room for an age of age cells and a space, on a line of its own if
// need be.
func wrapWork(is core.Issue, width, age int) (ref string, lines []string) {
	if width <= 0 {
		return "", []string{""}
	}
	ref = is.Repo.Name + "#" + strconv.Itoa(is.Number)
	ref = truncate(ref, min(ansi.StringWidth(ref), max(width/2, 1)))
	title := cleanLine(is.Title)
	if title == "" {
		lines = []string{""}
	} else {
		// A stand-in without breakpoints keeps the reference whole, so the
		// title's first line starts after it.
		refW := ansi.StringWidth(ref)
		text := strings.Repeat("x", refW) + " " + title
		lines = strings.Split(ansi.Wrap(text, width, ""), "\n")
		lines[0] = lines[0][min(refW, len(lines[0])):]
		for i := range lines {
			lines[i] = strings.TrimSpace(lines[i])
		}
	}
	last := ansi.StringWidth(lines[len(lines)-1])
	if len(lines) == 1 {
		last += ansi.StringWidth(ref) + 1
	}
	if last+1+age > width {
		lines = append(lines, "")
	}
	return ref, lines
}

// lines is how many lines row i takes.
func (t *workTab) lines(i int) int {
	if r := &t.rows[i]; r.hit != nil {
		return len(r.lines)
	}
	return 1
}

// span is how many lines rows first to last take.
func (t *workTab) span(first, last int) int {
	n := 0
	for i := first; i <= last; i++ {
		n += t.lines(i)
	}
	return n
}

// scroll shows the whole item under the cursor in height lines, and fills
// the window from the bottom.
func (t *workTab) scroll(height int) {
	if height <= 0 || len(t.rows) == 0 {
		return
	}
	if t.sel < t.items {
		t.top = min(t.top, t.sel)
		for t.top < t.sel && t.span(t.top, t.sel) > height {
			t.top++
		}
	}
	// The last rows fill the window rather than leave it half empty.
	last := len(t.rows) - 1
	for t.top > 0 && t.span(t.top-1, last) <= height {
		t.top--
	}
	t.top = min(max(t.top, 0), last)
}
