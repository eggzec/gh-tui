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

func TestWrapWork(t *testing.T) {
	issue := func(repo string, number int, title string) core.Issue {
		r, _ := core.ParseRepoRef(repo)
		return core.Issue{Repo: r, Number: number, Title: title}
	}
	tests := []struct {
		name  string
		issue core.Issue
		width int
		ref   string
		lines []string
	}{
		{"fits", issue("o/cli", 12, "Fix it"), 40, "cli#12", []string{"Fix it"}},
		{
			"wraps under the text", issue("o/permit", 48, "Implement license key generation with batch metadata and error handling"), 40,
			"permit#48", []string{"Implement license key", "generation with batch metadata and error", "handling"},
		},
		{
			"leaves room for the age", issue("o/slk", 239, "abcdefghij abcdefghij abcdefghij"), 40,
			"slk#239", []string{"abcdefghij abcdefghij abcdefghij", ""},
		},
		{"no title", issue("o/cli", 1, ""), 40, "cli#1", []string{""}},
		{"a long name is cut", issue("o/a-very-long-repository-name", 7, "Title"), 20, "a-very-lo…", []string{"Title"}},
		// A word too long for the rest of the first line starts the next.
		{"a long word is broken", issue("o/x", 1, strings.Repeat("y", 30)), 20, "x#1", []string{"", strings.Repeat("y", 20), strings.Repeat("y", 10)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, lines := wrapWork(tt.issue, tt.width, 3, "…")
			if ref != tt.ref || strings.Join(lines, "|") != strings.Join(tt.lines, "|") {
				t.Fatalf("wrapWork = %q, %q; want %q, %q", ref, lines, tt.ref, tt.lines)
			}
			for i, l := range lines {
				w := ansi.StringWidth(l)
				if i == 0 {
					w += ansi.StringWidth(ref) + 1
				}
				if i == len(lines)-1 {
					w += 1 + 3
				}
				if w > tt.width {
					t.Errorf("line %d %q takes %d cells, more than %d", i, l, w, tt.width)
				}
			}
		})
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

func TestWorkWraps(t *testing.T) {
	svc := newFake()
	svc.work = longWork()
	s := newSection(t, svc, nil, 56, 30, WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, s, "3")
	want := [][]string{{
		// The full titles of the tabs don't fit in 54 cells.
		" Reviews 1  Mine 3  Assigned 1",
		"> O permit#48 Implement license key generation with",
		">   batch metadata and error handling              6mo",
	}, {
		" Reviews 1  Mine 3  Assigned 1",
		"> O slk#239 fix(ui): preserve scroll position in",
		">   messages.Model on resize; throttle the redraws  1h",
		"  O playground#3 Add initial sample README content 9mo",
		"  O gh-tui#71 feat(dashboard): wrap the work waiting",
		"    on you under its text, and lay the repositories",
		"    out in columns                                  2h",
	}, {
		" Reviews 1  Mine 3  Assigned 1",
		"> o gh-tui#70 The dashboard cuts the titles of the",
		">   work waiting on you                             3d",
	}}
	for i, want := range want {
		got := strings.Join(workLines(s), "\n")
		if got != strings.Join(want, "\n") {
			t.Errorf("tab %d shows\n%s\nwant\n%s", i, got, strings.Join(want, "\n"))
		}
		press(t, s, "]")
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
	press(t, s, "right", "[")
	if l.cur != 1 || l.current().sel != 2 {
		t.Fatalf("back on tab %d item %d, want the third of your pull requests still", l.cur, l.current().sel)
	}
	if hit, _ := l.selected(); hit.Issue.Number != 71 {
		t.Errorf("the cursor is on #%d, want #71", hit.Issue.Number)
	}
	// Tabs wrap around both ways, with the keys of the owners.
	press(t, s, "left", "left")
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
		// The selected item shows whole: its every line has the cursor,
		// and the last of them its age.
		sel := tab.rows[tab.sel]
		marked := 0
		for _, l := range lines {
			if strings.HasPrefix(l, ">") {
				marked++
			}
		}
		if marked != len(sel.lines) {
			t.Errorf("step %d: %d lines have the cursor, want the %d of %s\n%s", step, marked, len(sel.lines), sel.ref, strings.Join(lines, "\n"))
		}
		// No item is cut at the bottom: the last line is the end of an
		// item, which has its age.
		if last := strings.TrimSpace(lines[len(lines)-1]); !strings.HasSuffix(last, "1h") && !strings.HasSuffix(last, "2h") && !strings.HasSuffix(last, "9mo") {
			t.Errorf("step %d: the pane ends in the middle of an item:\n%s", step, strings.Join(lines, "\n"))
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
	for _, want := range []string{"O a#5 Open", "D a#6 Draft", "M a#3 Merged", "X a#4 Declined", "o a#7 Open", "x a#1 Closed", "- a#2 Closed"} {
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
