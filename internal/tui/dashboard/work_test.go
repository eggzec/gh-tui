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
			ref, lines := wrapWork(tt.issue, tt.width, 3)
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
	got := strings.Join(workLines(s), "\n")
	want := strings.Join([]string{
		" Review requests 1",
		"▌ O permit#48 Implement license key generation with",
		"▌   batch metadata and error handling              6mo",
		"",
		" Your pull requests 3",
		"  O slk#239 fix(ui): preserve scroll position in",
		"    messages.Model on resize; throttle the redraws  1h",
		"  O playground#3 Add initial sample README content 9mo",
		"  O gh-tui#71 feat(dashboard): wrap the work waiting",
		"    on you under its text, and lay the repositories",
		"    out in columns                                  2h",
		"",
		" Assigned issues 1",
		"  o gh-tui#70 The dashboard cuts the titles of the",
		"    work waiting on you                             3d",
	}, "\n")
	if got != want {
		t.Errorf("the work pane shows\n%s\nwant\n%s", got, want)
	}
}

func TestWorkScrollsByWholeItems(t *testing.T) {
	svc := newFake()
	svc.work = longWork()
	s := newSection(t, svc, nil, 56, 12, WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, s, "3")
	_, h := s.boxes[workPane].w-2, s.boxes[workPane].h-2
	for step := range 2 * len(s.tasks.items) {
		key := "down"
		if step >= len(s.tasks.items) {
			key = "up"
		}
		lines := workLines(s)
		if len(lines) > h {
			t.Fatalf("step %d: %d lines in a pane of %d", step, len(lines), h)
		}
		// The selected item shows whole: its every line has the cursor,
		// and the last of them its age.
		sel := s.tasks.rows[s.tasks.items[s.tasks.sel]]
		marked := 0
		for _, l := range lines {
			if strings.HasPrefix(l, "▌") {
				marked++
			}
		}
		if marked != len(sel.lines) {
			t.Errorf("step %d: %d lines have the cursor, want the %d of %s\n%s", step, marked, len(sel.lines), sel.ref, strings.Join(lines, "\n"))
		}
		// No item is cut at the bottom: the last line is a header, a note
		// or the end of an item, which has its age.
		if last := strings.TrimSpace(lines[len(lines)-1]); !strings.HasSuffix(last, "1h") && !strings.HasSuffix(last, "2h") &&
			!strings.HasSuffix(last, "3d") && !strings.HasSuffix(last, "6mo") && !strings.HasSuffix(last, "9mo") && !strings.Contains(last, "Your pull requests") && !strings.Contains(last, "Assigned issues") {
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
	for _, want := range []string{"O a#5 Open", "D a#6 Draft", "M a#3 Merged", "X a#4 Declined", "o a#7 Open", "x a#1 Closed", "- a#2 Closed"} {
		if !strings.Contains(view, want) {
			t.Errorf("the work pane doesn't show %q:\n%s", want, view)
		}
	}
}
