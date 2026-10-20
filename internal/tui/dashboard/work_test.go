package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// A reference too long for its column loses the end of its repository,
// never its number.
func TestWorkCell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ref   string
		width int
		want  string
	}{
		{"o/cli#12", 12, "o/cli#12"},
		{"o/a-very-long-repository-name#7", 12, "o/a-very-…#7"},
		{"o/cli#12345", 4, "#12…"},
		{"o/cli#12", 0, ""},
	}
	for _, tt := range tests {
		got := cell(tt.ref, tt.width, "…")
		if got != tt.want {
			t.Errorf("cell(%q, %d) = %q, want %q", tt.ref, tt.width, got, tt.want)
		}
		if w := ansi.StringWidth(got); w > tt.width {
			t.Errorf("cell(%q, %d) takes %d cells", tt.ref, tt.width, w)
		}
	}
}

// longWork is work whose titles wrap in a narrow pane.
func longWork() core.Work {
	long := func(kind core.SearchKind, repo string, n int, title string, ago time.Duration) core.SearchHit {
		return hit(kind, repo, n, title, false, ago)
	}
	return core.Work{
		ReviewRequested: core.WorkList{Count: 1, Items: []core.SearchHit{
			long(core.SearchPulls, "eggzec/permit", 48, "Implement license key generation with batch metadata and error handling", 180*24*time.Hour),
		}},
		Authored: core.WorkList{Count: 3, Items: []core.SearchHit{
			long(core.SearchPulls, "eggzec/slk", 239, "fix(ui): preserve scroll position in messages.Model on resize; throttle the redraws", time.Hour),
			long(core.SearchPulls, "eggzec/playground", 3, "Add initial sample README content", 270*24*time.Hour),
			long(core.SearchPulls, "eggzec/gh-tui", 71, "feat(dashboard): wrap the work waiting on you under its text, and lay the repositories out in columns", 2*time.Hour),
		}},
		Assigned: core.WorkList{Count: 1, Items: []core.SearchHit{
			long(core.SearchIssues, "eggzec/gh-tui", 70, "The dashboard cuts the titles of the work waiting on you", 3*24*time.Hour),
		}},
	}
}

// workLines returns the lines of the work pane inside its frame, without
// styles.
func workLines(s *Section) []string {
	b := s.boxes[workPane]
	body := s.workBody(b.w-2, b.h-2)
	for i := range body {
		body[i] = ansi.Strip(body[i])
	}
	return body
}

// An item takes one line, in columns under a header: the owner in its
// reference, a long title cut at its end, and the age in its own column.
func TestWorkColumns(t *testing.T) {
	t.Parallel()
	svc := newFake()
	svc.work = longWork()
	s := newSection(t, svc, nil, 220, 30, WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, s, "3", "]")
	lines := workLines(s)
	// The tabs, the header, and a line for each of the three items.
	if len(lines) != 5 {
		t.Fatalf("the pane shows %d lines, want 5:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	header := lines[1]
	for _, l := range lines[1:] {
		if w := ansi.StringWidth(l); w > s.boxes[workPane].w-2 {
			t.Errorf("%q takes %d cells, more than the pane", l, w)
		}
	}
	titleAt, ageAt := strings.Index(header, titleHeader), strings.Index(header, ageHeader)
	if strings.Index(header, refHeader) != workIndent || titleAt < 0 || ageAt < titleAt {
		t.Fatalf("the header %q doesn't name the columns in order", header)
	}
	rows := lines[2:]
	for i, want := range []struct{ ref, title, age string }{
		{"eggzec/slk#239", "fix(ui): preserve scroll position in messages.Model on resize; throttle the redraws", "1h"},
		{"eggzec/playground#3", "Add initial sample README content", "9mo"},
		{"eggzec/gh-tui#71", "feat(dashboard): wrap the work waiting on you under its text, and lay the repositories out in columns", "2h"},
	} {
		row := rows[i]
		if !strings.HasPrefix(row[workIndent:], want.ref) {
			t.Errorf("row %d is %q, want it to start with the reference %q", i, row, want.ref)
		}
		// The titles line up under the header's, and the ages under theirs.
		if !strings.HasPrefix(row[titleAt:], strings.TrimRight(want.title[:min(len(want.title), 10)], " ")) {
			t.Errorf("row %d has %q where the title column starts, want %q", i, row[titleAt:], want.title)
		}
		if !strings.HasPrefix(row[ageAt:], want.age) {
			t.Errorf("row %d has %q where the age column starts, want %q", i, row[ageAt:], want.age)
		}
	}
	// A title too long for its column ends in the ellipsis, on the line.
	long := rows[2]
	if !strings.Contains(long[titleAt:ageAt], "...") {
		t.Errorf("the long title isn't cut with an ellipsis: %q", long)
	}
	if strings.Contains(strings.Join(lines, "\n"), "out in columns") {
		t.Errorf("the long title wasn't cut:\n%s", strings.Join(lines, "\n"))
	}
}

// A narrow pane cuts the repository of a reference and keeps its number.
func TestWorkNarrowKeepsNumbers(t *testing.T) {
	t.Parallel()
	svc := newFake()
	svc.work = longWork()
	s := newSection(t, svc, nil, 44, 30, WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, s, "3", "]")
	lines := workLines(s)
	for _, want := range []string{"#239", "#3", "#71"} {
		if !strings.Contains(strings.Join(lines, "\n"), want+" ") {
			t.Errorf("the narrow pane lost %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > s.boxes[workPane].w-2 {
			t.Errorf("%q takes %d cells, more than the pane", l, w)
		}
	}
}

// The selected item is bold, with the cursor in its gutter and the accent on
// its title, as in the other lists.
func TestWorkSelectedStyle(t *testing.T) {
	t.Parallel()
	svc := newFake()
	svc.work = longWork()
	s := newSection(t, svc, nil, 100, 30, WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, s, "3", "]")
	l := &s.tasks
	tab := l.current()
	sel := s.workItem(&tab.rows[0], true, true)
	plain := s.workItem(&tab.rows[1], false, true)
	if !strings.HasPrefix(ansi.Strip(sel), ">") || strings.HasPrefix(ansi.Strip(plain), ">") {
		t.Errorf("the cursor is on the wrong rows: %q, %q", ansi.Strip(sel), ansi.Strip(plain))
	}
	if !strings.Contains(sel, "\x1b[1") {
		t.Errorf("the selected row isn't bold: %q", sel)
	}
	if strings.Contains(plain, "\x1b[1m") {
		t.Errorf("a row that isn't selected is bold: %q", plain)
	}
}

func TestWorkTabs(t *testing.T) {
	svc := newFake()
	svc.work = longWork()
	s := newSection(t, svc, nil, 140, 38)
	press(t, s, "3")
	l := &s.tasks
	if l.cur != 0 {
		t.Fatalf("on tab %d, want the review requests", l.cur)
	}
	// Each tab keeps its cursor.
	press(t, s, "]", "down", "down")
	if l.cur != 1 || l.current().sel != 2 {
		t.Fatalf("on tab %d item %d, want the third of your pull requests", l.cur, l.current().sel)
	}
	press(t, s, "]", "[")
	if l.cur != 1 || l.current().sel != 2 {
		t.Fatalf("back on tab %d item %d, want the third of your pull requests still", l.cur, l.current().sel)
	}
	if hit, _ := l.selected(); hit.Issue.Number != 71 {
		t.Errorf("the cursor is on #%d, want #71", hit.Issue.Number)
	}
	// Tabs wrap around both ways.
	press(t, s, "[", "[")
	if l.cur != 2 {
		t.Errorf("two tabs back from the second is tab %d, want the last", l.cur)
	}
	press(t, s, "]")
	if l.cur != 0 {
		t.Errorf("the tab after the last is %d, want the first", l.cur)
	}
	// New work keeps the tab picked and the cursor of each.
	press(t, s, "]")
	s.Update(loadedMsg{id: s.id, gen: s.gen, kind: kindWork, value: longWork()})
	if l.cur != 1 || l.current().sel != 2 {
		t.Errorf("after new work on tab %d item %d, want them kept", l.cur, l.current().sel)
	}
	// The other panes keep their keys.
	press(t, s, "2", "]")
	if l.cur != 1 || s.repos.cur != 1 {
		t.Errorf("] in the repositories moved the work to tab %d and the owners to %d", l.cur, s.repos.cur)
	}
}

func TestWorkOpensOnTheFirstTabWithItems(t *testing.T) {
	svc := newFake()
	svc.work = longWork()
	svc.work.ReviewRequested = core.WorkList{}
	s := newSection(t, svc, nil, 140, 38)
	if s.tasks.cur != 1 {
		t.Errorf("on tab %d, want your pull requests, the first with items", s.tasks.cur)
	}
	svc.work = core.Work{}
	s = newSection(t, svc, nil, 140, 38)
	if s.tasks.cur != 0 || !strings.Contains(screen(s), "No review requests.") {
		t.Errorf("with no work on tab %d, want the first, which says so:\n%s", s.tasks.cur, screen(s))
	}
}

func TestWorkScrollsByWholeItems(t *testing.T) {
	svc := newFake()
	svc.work = longWork()
	s := newSection(t, svc, nil, 56, 9, WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, s, "3", "]")
	h := s.boxes[workPane].h - 2
	tab := s.tasks.current()
	for step := range 2 * tab.items {
		key := "down"
		if step >= tab.items {
			key = "up"
		}
		lines := workLines(s)
		if len(lines) > h {
			t.Fatalf("step %d: %d lines in a pane of %d", step, len(lines), h)
		}
		// The selected item shows, once, under the cursor.
		sel := tab.rows[tab.sel]
		marked := 0
		for _, l := range lines {
			if strings.HasPrefix(l, ">") {
				marked++
				if !strings.Contains(l, sel.ref[strings.LastIndex(sel.ref, "#"):]) || !strings.Contains(l, sel.ref[:6]) {
					t.Errorf("step %d: the cursor is on %q, want %s", step, l, sel.ref)
				}
			}
		}
		if marked != 1 {
			t.Errorf("step %d: %d lines have the cursor, want 1\n%s", step, marked, strings.Join(lines, "\n"))
		}
		press(t, s, key)
	}
}

func TestWorkStates(t *testing.T) {
	svc := newFake()
	closed := hit(core.SearchIssues, "o/a", 1, "Closed", false, time.Hour)
	closed.Issue.State = core.StateClosed
	notPlanned := closed
	notPlanned.Issue.Number, notPlanned.Issue.Reason = 2, core.ReasonNotPlanned
	merged := hit(core.SearchPulls, "o/a", 3, "Merged", false, time.Hour)
	merged.Issue.State = core.StateMerged
	declined := hit(core.SearchPulls, "o/a", 4, "Declined", false, time.Hour)
	declined.Issue.State = core.StateClosed
	svc.work = core.Work{
		ReviewRequested: core.WorkList{Count: 3, Items: []core.SearchHit{
			hit(core.SearchPulls, "o/a", 5, "Open", false, time.Hour), hit(core.SearchPulls, "o/a", 6, "Draft", true, time.Hour), merged, declined,
		}},
		Assigned: core.WorkList{Count: 3, Items: []core.SearchHit{hit(core.SearchIssues, "o/a", 7, "Open", false, time.Hour), closed, notPlanned}},
	}
	s := newSection(t, svc, nil, 56, 30, WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, s, "3")
	view := strings.Join(workLines(s), "\n")
	press(t, s, "]", "]")
	view += strings.Join(workLines(s), "\n")
	for _, want := range []string{"O o/a#5 Open", "D o/a#6 Draft", "M o/a#3 Merged", "X o/a#4 Declined", "o o/a#7 Open", "x o/a#1 Closed", "- o/a#2 Closed"} {
		if !strings.Contains(view, want) {
			t.Errorf("the work pane doesn't show %q:\n%s", want, view)
		}
	}
}

// A list whose search GitHub refused, while it answered the others, says
// it can't be read, as another refused pane does, not that it is empty,
// and its tab counts it as unknown.
func TestWorkRefusedList(t *testing.T) {
	svc := newFake()
	svc.work = longWork()
	svc.work.Authored = core.WorkList{Refused: true}
	s := newSection(t, svc, nil, 140, 38)
	press(t, s, "3", "]")
	if s.tasks.cur != 1 {
		t.Fatalf("on tab %d, want your pull requests", s.tasks.cur)
	}
	view := ansi.Strip(s.View())
	want, _ := s.say("load your pull requests", core.ErrForbidden)
	if !strings.Contains(view, "Mine ?") && !strings.Contains(view, "Your pull requests ?") {
		t.Errorf("the tab doesn't count the refused list as unknown:\n%s", view)
	}
	words := strings.Fields(want)
	if len(words) < 3 || !strings.Contains(strings.Join(strings.Fields(view), " "), strings.Join(words[:3], " ")) {
		t.Errorf("the pane doesn't say %q:\n%s", want, view)
	}
	if strings.Contains(view, workLists[1].empty) {
		t.Errorf("the pane says the refused list is empty:\n%s", view)
	}
}
