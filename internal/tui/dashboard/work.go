package dashboard

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// workRow is a row of the work pane: the header of a list, one of its
// items, or what stands in for them.
type workRow struct {
	// header names a list, with its count.
	header string
	count  int
	hit    *core.SearchHit
	// note is shown when a list is empty, or holds more than it lists.
	note string

	// ref and lines are an item wrapped to the pane: ref, such as
	// "cli#12", starts the first line, and lines hold its title, the first
	// after ref. The last line leaves room for the age.
	ref   string
	lines []string
}

// workList is the work pane: the three lists of work waiting on the
// viewer, one after the other, with a cursor over their items. An item
// takes as many lines as its title needs, and the window scrolls by whole
// rows.
type workList struct {
	rows []workRow
	// items holds the index in rows of each item, in order.
	items []int
	sel   int
	// top is the first row on view.
	top int
	// width and height are the size of the pane, inside its frame.
	width, height int
	// now is the clock that ages are measured against.
	now func() time.Time
}

// workIndent is the room before the text of a work item: the gutter, and
// the state and its space.
const workIndent = 4

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
	l.wrap()
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

func (l *workList) resize(width, height int) {
	width, height = max(width, 0), max(height, 0)
	if width != l.width {
		l.width = width
		l.wrap()
	}
	l.height = height
	l.scroll()
}

// wrap breaks the items into lines of the pane's width.
func (l *workList) wrap() {
	for i := range l.rows {
		if r := &l.rows[i]; r.hit != nil {
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

// lines is how many lines row i takes when first is the first row on
// view: a header has a blank line above it, unless it is at the top.
func (l *workList) lines(i, first int) int {
	r := &l.rows[i]
	switch {
	case r.hit != nil:
		return len(r.lines)
	case r.header != "" && i > first:
		return 2
	default:
		return 1
	}
}

// span is how many lines rows first to last take with first on top.
func (l *workList) span(first, last int) int {
	n := 0
	for i := first; i <= last; i++ {
		n += l.lines(i, first)
	}
	return n
}

// scroll shows the whole item under the cursor, with the header of its
// list when there is room, and fills the window from the bottom.
func (l *workList) scroll() {
	if l.height <= 0 || len(l.rows) == 0 {
		return
	}
	if l.sel < len(l.items) {
		row := l.items[l.sel]
		first := row
		// The header of a list comes into view with its first item.
		if row > 0 && l.rows[row-1].header != "" {
			first--
		}
		if first < l.top {
			l.top = first
		}
		for l.top < row && l.span(l.top, row) > l.height {
			l.top++
		}
	}
	// The last rows fill the window rather than leave it half empty.
	last := len(l.rows) - 1
	for l.top > 0 && l.span(l.top-1, last) <= l.height {
		l.top--
	}
	l.top = min(max(l.top, 0), last)
}
