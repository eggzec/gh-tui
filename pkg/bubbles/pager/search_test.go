package pager

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// bigText is more than syncLimit of numbered lines, so it is searched in
// the background.
func bigText(tb testing.TB) string {
	tb.Helper()
	text := numbered(30_000)
	if len(text) < syncLimit {
		tb.Fatalf("%d bytes is searched at once", len(text))
	}
	return text
}

// typeSearch opens the input, types query and confirms it, and returns
// the command that runs the search, if any.
func typeSearch(tb testing.TB, m Model, query string) (Model, tea.Cmd) {
	tb.Helper()
	m, _ = keys(tb, m, "/")
	m = typeText(tb, m, query)
	return m.Update(enter)
}

func TestSearchInBackground(t *testing.T) {
	text := bigText(t)
	m := open(t, "big.txt", text, WithSize(40, 11))
	m, run := typeSearch(t, m, "line x ")
	if run == nil {
		t.Fatal("a large file was searched at once")
	}
	if m.Matches() != 0 || m.KeyMap().Next.Enabled() || !strings.Contains(plain(m), "searching…") {
		t.Errorf("while running: %d matches, next enabled %v, status %q; want none, and searching…",
			m.Matches(), m.KeyMap().Next.Enabled(), lastLine(plain(m)))
	}
	// The window shows the matches before the search is done.
	if len(m.hits.lines) == 0 {
		t.Error("no matches in the window while running")
	}

	// The search reads the lines while the pager scrolls and renders.
	var wg sync.WaitGroup
	var msg tea.Msg
	wg.Go(func() { msg = run() })
	for range 50 {
		m, _ = keys(t, m, "j")
		_ = m.View()
	}
	wg.Wait()
	m, _ = m.Update(msg)
	want := strings.Count(text, "line x ")
	if m.Matches() != want {
		t.Fatalf("%d matches, want %d", m.Matches(), want)
	}
	// The window stays where the user scrolled it while the search ran.
	status := fmt.Sprintf("%d matches", want)
	if !m.KeyMap().Next.Enabled() || m.top != 50 || !strings.Contains(plain(m), status) {
		t.Errorf("done: next enabled %v, top %d, status %q; want %s at line 51",
			m.KeyMap().Next.Enabled(), m.top, lastLine(plain(m)), status)
	}
	// n goes to the first match from there, on line 51.
	m, _ = keys(t, m, "n")
	status = fmt.Sprintf("match 8/%d", want)
	if m.search.curLine != 50 || m.top != 50 || !strings.Contains(plain(m), status) {
		t.Errorf("n: match on line %d at top %d, status %q; want %s on line 51",
			m.search.curLine, m.top, lastLine(plain(m)), status)
	}
}

// A search the user doesn't scroll away from jumps to its first match
// when it is done.
func TestSearchInBackgroundJumps(t *testing.T) {
	text := bigText(t)
	want := strings.Count(text, "line x ")
	m := open(t, "big.txt", text, WithSize(40, 11))
	m, _ = keys(t, m, "G")
	m, run := typeSearch(t, m, "line x ")
	m, _ = m.Update(run())
	// The first match from the top of the window at the end is the last.
	if m.search.cur != want-1 || m.search.curLine != 29_996 {
		t.Errorf("match %d on line %d, want the last, on line 29997", m.search.cur, m.search.curLine+1)
	}
}

// A parent that opens the pager on a line searches from it, whether the
// search runs at once or in the background.
func TestSetSearchFromTheMarkedLine(t *testing.T) {
	for _, text := range []string{numbered(200), bigText(t)} {
		m := open(t, "lines.txt", text, WithSize(40, 11))
		m.GoToLine(150)
		top := m.top
		if cmd := m.SetSearch("line x "); cmd != nil {
			m, _ = m.Update(cmd())
		}
		// "line x " is on every seventh line from the second: the first
		// from line 150 is line 156, which is in view.
		if m.search.curLine != 155 || m.top != top {
			t.Errorf("%d bytes: match on line %d at top %d, want the first from line 150 at top %d",
				len(text), m.search.curLine+1, m.top, top)
		}
	}
}

// A line that matches all over costs no more than maxLineMatches matches,
// and the window keeps only those it shows.
func TestSearchLongLine(t *testing.T) {
	text := strings.Repeat("a", 1<<20) + "\n"
	for _, query := range []string{"a", "z*"} {
		m := open(t, "min.js", text, WithSize(80, 11))
		var found Model
		start := time.Now()
		// A few passes over the line, each capped; without the cap, "a"
		// takes a million allocations.
		allocs := testing.AllocsPerRun(1, func() {
			var cmd tea.Cmd
			if found, cmd = typeSearch(t, m, query); cmd != nil {
				found, _ = found.Update(cmd())
			}
			_ = found.View()
		})
		if allocs > 20*maxLineMatches {
			t.Errorf("%q: %v allocations, want at most %d", query, allocs, 20*maxLineMatches)
		}
		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("%q: took %v", query, took)
		}
		want := maxLineMatches
		if query == "z*" {
			// Empty matches can't be shown, so they don't count.
			want = 0
		}
		if found.Matches() != want {
			t.Errorf("%q: %d matches, want %d", query, found.Matches(), want)
		}
		if n := len(found.hits.lines); n > 0 && len(found.hits.lines[0].ranges) > found.Width() {
			t.Errorf("%q: the window keeps %d ranges", query, len(found.hits.lines[0].ranges))
		}
	}
}

func TestSearchDropsStaleResults(t *testing.T) {
	m := open(t, "big.txt", bigText(t), WithSize(40, 11))
	m, first := typeSearch(t, m, "line x ")
	stale := first()
	m, second := typeSearch(t, m, "line xx ")
	m, _ = m.Update(stale)
	if m.Matches() != 0 || !m.search.running {
		t.Fatalf("took the results of an older search: %d matches", m.Matches())
	}
	m, _ = m.Update(second())
	if m.Query() != "line xx " || m.Matches() != strings.Count(bigText(t), "line xx ") {
		t.Errorf("query %q with %d matches, want the second search's", m.Query(), m.Matches())
	}
}

func TestSearchCancelled(t *testing.T) {
	tests := []struct {
		name string
		stop func(*Model)
	}{
		{name: "new content", stop: func(m *Model) { m.SetContent("other.txt", "other") }},
		{name: "loading", stop: func(m *Model) { m.SetLoading("other.txt") }},
		{name: "a new search", stop: func(m *Model) { m.SetSearch("line") }},
		{name: "cleared", stop: func(m *Model) { m.SetSearch("") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "big.txt", bigText(t), WithSize(40, 11))
			m, run := typeSearch(t, m, "line x ")
			tt.stop(&m)
			if msg := run(); msg != nil {
				t.Errorf("the search ran on after it was cancelled: %T", msg)
			}
		})
	}
}

// The count comes from the lines, each of which may hold several matches.
func TestSearchCounts(t *testing.T) {
	m := open(t, "a.txt", "a a\nb\na\nb a a a\n", WithSize(20, 5))
	m, _ = typeSearch(t, m, "a")
	if m.Matches() != 6 {
		t.Fatalf("%d matches, want 6", m.Matches())
	}
	steps := []struct {
		key       string
		want      string
		line, nth int
	}{
		{key: "n", want: "match 2/6", line: 0, nth: 1},
		{key: "n", want: "match 3/6", line: 2, nth: 0},
		{key: "n", want: "match 4/6", line: 3, nth: 0},
		{key: "n", want: "match 5/6", line: 3, nth: 1},
		{key: "n", want: "match 6/6", line: 3, nth: 2},
		{key: "n", want: "match 1/6", line: 0, nth: 0},
		{key: "N", want: "match 6/6", line: 3, nth: 2},
	}
	for _, s := range steps {
		m, _ = keys(t, m, s.key)
		if got := lastLine(plain(m)); !strings.Contains(got, s.want) ||
			m.search.curLine != s.line || m.search.curNth != s.nth {
			t.Fatalf("after %s: %q at line %d match %d, want %s at %d match %d",
				s.key, got, m.search.curLine, m.search.curNth, s.want, s.line, s.nth)
		}
	}
}

func lastLine(v string) string {
	return v[strings.LastIndexByte(v, '\n')+1:]
}
