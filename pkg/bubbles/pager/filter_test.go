package pager

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// typeFilter opens the filter prompt, types pattern and confirms it, and
// returns the command that picks the lines, if any.
func typeFilter(tb testing.TB, m Model, pattern string) (Model, tea.Cmd) {
	tb.Helper()
	m, _ = keys(tb, m, "&")
	m = typeText(tb, m, pattern)
	return m.Update(enter)
}

// shownLines returns the numbers of the lines shown, counted from 1.
func shownLines(m Model) []int {
	out := make([]int, m.count())
	for p := range out {
		out[p] = m.at(p) + 1
	}
	return out
}

func TestFilter(t *testing.T) {
	// "line x" is on every line but every seventh from the first, and
	// "line x " on every seventh from the second.
	text := numbered(100)
	tests := []struct {
		name          string
		keys          []string
		wantFilter    string
		wantShown     int
		wantFirst     int
		wantQuery     string
		wantMatches   int
		wantNote      string
		wantStatus    string
		wantCapturing bool
		wantClose     bool
	}{
		{name: "ampersand opens the prompt", keys: []string{"&"}, wantShown: 100, wantFirst: 1, wantCapturing: true},
		{name: "enter filters", keys: []string{"&", "line x ", "enter"},
			wantFilter: "line x ", wantShown: 15, wantFirst: 2, wantStatus: "filtered 15/100"},
		{name: "bang keeps the lines that don't match", keys: []string{"&", "!line x", "enter"},
			wantFilter: "!line x", wantShown: 15, wantFirst: 1, wantStatus: "filtered 15/100"},
		{name: "patterns are regexps", keys: []string{"&", "^line x{5} ", "enter"},
			wantFilter: "^line x{5} ", wantShown: 14, wantFirst: 6},
		{name: "an empty line shows every line", keys: []string{"&", "line x ", "enter", "&", "enter"},
			wantShown: 100, wantFirst: 2},
		{name: "a bang alone shows every line", keys: []string{"&", "line x ", "enter", "&", "!", "enter"},
			wantShown: 100, wantFirst: 2},
		{name: "an invalid pattern keeps the filter", keys: []string{"&", "line x ", "enter", "&", "(", "enter"},
			wantFilter: "line x ", wantShown: 15, wantFirst: 2, wantNote: "Invalid pattern: missing closing )"},
		{name: "a pattern that keeps nothing keeps the filter", keys: []string{"&", "line x ", "enter", "&", "kiwi", "enter"},
			wantFilter: "line x ", wantShown: 15, wantFirst: 2, wantNote: "Pattern not found"},
		{name: "a new filter replaces the old", keys: []string{"&", "line x ", "enter", "&", "line xx ", "enter"},
			wantFilter: "line xx ", wantShown: 14, wantFirst: 3},
		{name: "search finds only the lines shown", keys: []string{"&", "line x ", "enter", "/", "1", "enter"},
			wantFilter: "line x ", wantShown: 15, wantFirst: 2, wantQuery: "1", wantMatches: 3,
			wantStatus: "filtered 15/100  match 1/3"},
		{name: "a filter runs the search again", keys: []string{"/", "1", "enter", "&", "line x ", "enter"},
			wantFilter: "line x ", wantShown: 15, wantFirst: 2, wantQuery: "1", wantMatches: 3,
			wantStatus: "filtered 15/100  3 matches"},
		{name: "a filter that hides every match clears the search", keys: []string{"/", "xxxxxx", "enter", "&", "line x ", "enter"},
			wantFilter: "line x ", wantShown: 15, wantFirst: 2, wantNote: "Pattern not found"},
		{name: "esc clears the search first", keys: []string{"&", "line x ", "enter", "/", "1", "enter", "esc"},
			wantFilter: "line x ", wantShown: 15, wantFirst: 2},
		{name: "then the filter", keys: []string{"&", "line x ", "enter", "/", "1", "enter", "esc", "esc"},
			wantShown: 100, wantFirst: 2},
		{name: "then closes", keys: []string{"&", "line x ", "enter", "esc", "esc"},
			wantShown: 100, wantFirst: 2, wantClose: true},
		{name: "esc at the prompt keeps the filter", keys: []string{"&", "line x ", "enter", "&", "kiwi", "esc"},
			wantFilter: "line x ", wantShown: 15, wantFirst: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "lines.txt", text, WithSize(80, 11))
			m, msg := enterAll(t, m, tt.keys...)
			if m.Filter() != tt.wantFilter || m.Shown() != tt.wantShown {
				t.Errorf("filter %q showing %d lines, want %q showing %d", m.Filter(), m.Shown(), tt.wantFilter, tt.wantShown)
			}
			if got := m.topLine() + 1; got != tt.wantFirst {
				t.Errorf("top line %d, want %d", got, tt.wantFirst)
			}
			if m.Query() != tt.wantQuery || m.Matches() != tt.wantMatches {
				t.Errorf("query %q with %d matches, want %q with %d", m.Query(), m.Matches(), tt.wantQuery, tt.wantMatches)
			}
			if m.flash != tt.wantNote {
				t.Errorf("note %q, want %q", m.flash, tt.wantNote)
			}
			status := lastLine(plain(m))
			if tt.wantStatus != "" && !strings.Contains(status, tt.wantStatus) {
				t.Errorf("status %q, want %q in it", status, tt.wantStatus)
			}
			if tt.wantFilter == "" && strings.Contains(status, "filtered") {
				t.Errorf("status %q says filtered with no filter", status)
			}
			if m.Capturing() != tt.wantCapturing {
				t.Errorf("capturing %v, want %v", m.Capturing(), tt.wantCapturing)
			}
			if tt.wantCapturing && !strings.HasPrefix(status, "&") {
				t.Errorf("status %q isn't the filter prompt", status)
			}
			if _, ok := msg.(CloseMsg); ok != tt.wantClose {
				t.Errorf("sent %#v, want close %v", msg, tt.wantClose)
			}
		})
	}
}

// The lines shown keep their numbers, and n and N step through the
// matches among them only.
func TestFilterSearchSteps(t *testing.T) {
	m := open(t, "lines.txt", numbered(100), WithSize(80, 6))
	m, _ = enterAll(t, m, "&", "line x ", "enter")
	want := []int{2, 9, 16, 23, 30, 37, 44, 51, 58, 65, 72, 79, 86, 93, 100}
	if got := shownLines(m); !slices.Equal(got, want) {
		t.Fatalf("shown %v, want %v", got, want)
	}
	if first := strings.Fields(plain(m))[0]; first != "2" {
		t.Errorf("first line numbered %q, want 2", first)
	}
	m, _ = enterAll(t, m, "/", "9", "enter")
	// 9, 79 and 93 of those have a 9.
	lines := make([]int, 0, 4)
	for range 4 {
		lines = append(lines, m.search.curLine+1)
		m, _ = keys(t, m, "n")
	}
	if !slices.Equal(lines, []int{9, 79, 93, 9}) {
		t.Errorf("n went to lines %v, want 9, 79, 93, 9", lines)
	}
	// n went on to 79, and N goes back to 9 and around to 93.
	m, _ = keys(t, m, "N", "N")
	if m.search.curLine+1 != 93 {
		t.Errorf("N went to line %d, want 93", m.search.curLine+1)
	}
	// The line of the match is in the window, among the lines shown.
	if p := m.posOf(m.search.curLine); p < m.top || p > m.bottom() {
		t.Errorf("match at position %d, window %d to %d", p, m.top, m.bottom())
	}
}

// The window keeps the line at its top when a filter shows it, and else
// goes to the first line shown after it.
func TestFilterKeepsThePlace(t *testing.T) {
	m := open(t, "lines.txt", numbered(100), WithSize(80, 6))
	m, _ = keys(t, m, "d", "d", "d", "d", "d", "d", "d", "d", "d", "d")
	if m.topLine() != 20 {
		t.Fatalf("top line %d, want 21", m.topLine()+1)
	}
	m, _ = enterAll(t, m, "&", "line x ", "enter")
	if m.topLine()+1 != 23 {
		t.Errorf("filtered: top line %d, want 23, the first shown after 21", m.topLine()+1)
	}
	m, _ = keys(t, m, "j", "esc")
	if m.topLine()+1 != 30 {
		t.Errorf("cleared: top line %d, want 30, where it was", m.topLine()+1)
	}
}

func TestFilterInBackground(t *testing.T) {
	text := bigText(t)
	m := open(t, "big.txt", text, WithSize(40, 11))
	m, run := typeFilter(t, m, "line x ")
	if run == nil {
		t.Fatal("a large file was filtered at once")
	}
	if m.Shown() != 30_000 || !strings.Contains(plain(m), "filtering…") || !m.KeyMap().Cancel.Enabled() {
		t.Errorf("while running: %d lines shown, status %q, cancel enabled %v; want all, filtering…, and cancel",
			m.Shown(), lastLine(plain(m)), m.KeyMap().Cancel.Enabled())
	}
	// The filter reads the lines while the pager scrolls and renders.
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
	status := fmt.Sprintf("filtered %d/30000", want)
	if m.Shown() != want || !strings.Contains(plain(m), status) {
		t.Fatalf("done: %d lines shown, status %q; want %d and %s", m.Shown(), lastLine(plain(m)), want, status)
	}
	// The window stays on line 51, which the filter shows.
	if m.topLine()+1 != 51 {
		t.Errorf("top line %d, want 51", m.topLine()+1)
	}
	m, _ = keys(t, m, "G")
	if m.topLine()+1 != 29_934 || m.at(m.bottom())+1 != 29_997 {
		t.Errorf("G: lines %d to %d, want 29934 to 29997", m.topLine()+1, m.at(m.bottom())+1)
	}
}

// A search over a filter in the background counts the lines shown only,
// and so does the search run again once a filter lands.
func TestFilterSearchInBackground(t *testing.T) {
	text := bigText(t)
	m := open(t, "big.txt", text, WithSize(40, 11))
	m, run := typeSearch(t, m, "1")
	m, _ = m.Update(run())
	m, run = typeFilter(t, m, "line x ")
	// The search shown stays until the filter lands.
	before := m.Matches()
	m, research := m.Update(run())
	if research == nil {
		t.Fatal("the filter didn't run the search again")
	}
	m, _ = m.Update(research())
	var want int
	for l := range strings.SplitSeq(text, "\n") {
		if strings.Contains(l, "line x ") {
			want += strings.Count(l, "1")
		}
	}
	if m.Matches() != want || m.Matches() >= before {
		t.Errorf("%d matches, want %d, fewer than the %d before", m.Matches(), want, before)
	}
	if m.search.cur != -1 {
		t.Errorf("the search jumped to match %d, want the window left where it was", m.search.cur)
	}
}

func TestFilterCancelled(t *testing.T) {
	tests := []struct {
		name string
		stop func(*Model) tea.Cmd
	}{
		{name: "esc", stop: func(m *Model) tea.Cmd {
			var cmd tea.Cmd
			*m, cmd = m.Update(escKey)
			return cmd
		}},
		{name: "an empty filter", stop: func(m *Model) tea.Cmd {
			var cmd tea.Cmd
			*m, cmd = typeFilter(t, *m, "")
			return cmd
		}},
		{name: "a new filter", stop: func(m *Model) tea.Cmd {
			var cmd tea.Cmd
			*m, cmd = typeFilter(t, *m, "line xx ")
			return cmd
		}},
		{name: "new content", stop: func(m *Model) tea.Cmd { return m.SetContent("other.txt", "other") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "big.txt", bigText(t), WithSize(40, 11))
			m, run := typeFilter(t, m, "line x ")
			_ = tt.stop(&m)
			if msg := run(); msg != nil {
				t.Errorf("the filter ran on after it was cancelled: %T", msg)
			}
			if m.projecting && m.want.filter.query == "line x " {
				t.Error("still filtering for the old pattern")
			}
		})
	}
}

// Esc stops a filter while it runs, and the one shown before stays.
func TestFilterCancelKeepsTheOld(t *testing.T) {
	m := open(t, "big.txt", bigText(t), WithSize(40, 11))
	m, run := typeFilter(t, m, "line x ")
	m, _ = m.Update(run())
	m, run = typeFilter(t, m, "line xx ")
	stale := run()
	m, _ = keys(t, m, "esc")
	m, _ = m.Update(stale)
	if m.Filter() != "line x " || m.projecting || strings.Contains(plain(m), "filtering…") {
		t.Errorf("filter %q, projecting %v, status %q; want the old one shown", m.Filter(), m.projecting, lastLine(plain(m)))
	}
	m, _ = keys(t, m, "esc")
	if m.Filter() != "" || m.Shown() != 30_000 {
		t.Errorf("the second esc left filter %q showing %d lines", m.Filter(), m.Shown())
	}
}

// Filtering many lines stays well within a frame's budget of Update: the
// work runs in the command.
func TestFilterHugeInput(t *testing.T) {
	text := numbered(200_000)
	m := open(t, "huge.txt", text, WithSize(80, 24))
	typed, _ := keys(t, m, "&")
	typed = typeText(t, typed, "line x ")
	allocs := testing.AllocsPerRun(1, func() {
		_, _ = typed.Update(enter)
	})
	if allocs > 50 {
		t.Errorf("confirming the filter costs Update %v allocations", allocs)
	}
	m, run := typeFilter(t, m, "7$")
	m, _ = m.Update(run())
	if m.Shown() != 20_000 {
		t.Errorf("%d lines shown, want 20000", m.Shown())
	}
	assertFits(t, m.View(), 80, 24)
}
