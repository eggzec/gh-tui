package cmdline

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestComplete(t *testing.T) {
	right := tea.KeyPressMsg{Code: tea.KeyRight}
	tests := []struct {
		name     string
		complete Complete
		// none sets no Complete at all.
		none    bool
		initial string
		keys    []tea.Msg
		// typed is typed after the keys.
		typed      string
		wantValue  string
		wantCursor int
		wantSel    int
		wantCands  int
	}{
		{
			name: "candidates show as the user types", initial: "goto cli",
			wantValue: "goto cli", wantCursor: 8, wantSel: -1, wantCands: 2,
		},
		{
			name: "tab inserts the first", initial: "goto gam", keys: []tea.Msg{tab},
			wantValue: "goto gammons/slk", wantCursor: 16, wantSel: 0, wantCands: 2,
		},
		{
			name: "tab again inserts the next", initial: "goto gam", keys: []tea.Msg{tab, tab},
			wantValue: "goto gammons/slk-web", wantCursor: 20, wantSel: 1, wantCands: 2,
		},
		{
			name: "past the last comes the line as typed", initial: "goto gam", keys: []tea.Msg{tab, tab, tab},
			wantValue: "goto gam", wantCursor: 8, wantSel: -1, wantCands: 2,
		},
		{
			name: "shift+tab starts from the last", initial: "goto charm", keys: []tea.Msg{shiftTab},
			wantValue: "goto charmbracelet/lipgloss", wantCursor: 27, wantSel: 2, wantCands: 3,
		},
		{
			name: "shift+tab steps back", initial: "goto charm", keys: []tea.Msg{tab, tab, shiftTab},
			wantValue: "goto charmbracelet/bubbletea", wantCursor: 28, wantSel: 0, wantCands: 3,
		},
		{
			name: "the span is replaced and the rest kept", initial: "goto gam --web",
			keys:      []tea.Msg{left, left, left, left, left, left, tab},
			wantValue: "goto gammons/slk --web", wantCursor: 16, wantSel: 0, wantCands: 2,
		},
		{
			name: "typing ends the cycle and completes again", initial: "goto gam",
			keys: []tea.Msg{tab}, typed: "-",
			wantValue: "goto gammons/slk-", wantCursor: 17, wantSel: -1, wantCands: 1,
		},
		{
			name: "moving the cursor completes again", initial: "goto cli/cli", keys: []tea.Msg{left, left, left, left},
			wantValue: "goto cli/cli", wantCursor: 8, wantSel: -1, wantCands: 2,
		},
		{
			name: "a move that goes nowhere keeps the cycle", initial: "goto gam", keys: []tea.Msg{tab, right},
			wantValue: "goto gammons/slk", wantCursor: 16, wantSel: 0, wantCands: 2,
		},
		{
			name: "tab without candidates does nothing", initial: "goto zzz", keys: []tea.Msg{tab},
			wantValue: "goto zzz", wantCursor: 8, wantSel: -1,
		},
		{
			name: "tab without a Complete does nothing", none: true, initial: "goto", keys: []tea.Msg{tab},
			wantValue: "goto", wantCursor: 4, wantSel: -1,
		},
		{
			name: "spans are bytes", initial: "café gam", keys: []tea.Msg{tab},
			wantValue: "café gammons/slk", wantCursor: 16, wantSel: 0, wantCands: 2,
		},
		{
			name: "bad spans are dropped", complete: badSpans, initial: "é",
			wantValue: "é", wantCursor: 1, wantSel: -1, wantCands: 1,
		},
		{
			name: "the good span of bad ones inserts", complete: badSpans, initial: "é", keys: []tea.Msg{tab},
			wantValue: "ok", wantCursor: 2, wantSel: 0, wantCands: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			complete := completeWords(repos...)
			switch {
			case tt.none:
				complete = nil
			case tt.complete != nil:
				complete = tt.complete
			}
			m := opened(t, tt.initial, WithSize(80, MaxHeight), WithComplete(complete))
			m, _ = press(t, m, tt.keys...)
			m = typeText(t, m, tt.typed)
			if v := m.Value(); v != tt.wantValue {
				t.Errorf("Value() = %q, want %q", v, tt.wantValue)
			}
			if p := m.input.Position(); p != tt.wantCursor {
				t.Errorf("cursor at %d, want %d", p, tt.wantCursor)
			}
			if s := m.Selected(); s != tt.wantSel {
				t.Errorf("Selected() = %d, want %d", s, tt.wantSel)
			}
			if n := len(m.Candidates()); n != tt.wantCands {
				t.Errorf("%d candidates, want %d: %v", n, tt.wantCands, m.Candidates())
			}
			wantHeight := 1
			if tt.wantCands > 0 {
				wantHeight = 2
			}
			if h := m.Height(); h != wantHeight {
				t.Errorf("Height() = %d, want %d", h, wantHeight)
			}
		})
	}
}

// badSpans offers one good candidate among spans that are out of order,
// past the end or inside a rune.
func badSpans(line string, _ int) []Candidate {
	return []Candidate{
		{Text: "a", Start: 1, End: 0},
		{Text: "b", Start: 0, End: len(line) + 1},
		{Text: "c", Start: 1, End: len(line)},
		{Text: "d", Start: -1, End: 0},
		{Text: "ok", Start: 0, End: len(line)},
	}
}

// The command line calls Complete with the line and the byte offset of the
// cursor, and keeps a copy of what it returns.
func TestCompleteArgs(t *testing.T) {
	type call struct {
		line   string
		cursor int
	}
	var calls []call
	out := []Candidate{{Text: "x", Start: 0, End: 0}}
	f := func(line string, cursor int) []Candidate {
		calls = append(calls, call{line, cursor})
		return out
	}
	m := opened(t, "", WithSize(80, MaxHeight), WithComplete(f))
	m = typeText(t, m, "ñé")
	m, _ = press(t, m, left)
	want := []call{{"", 0}, {"ñ", 2}, {"ñé", 4}, {"ñé", 2}}
	if !slices.Equal(calls, want) {
		t.Errorf("calls %v, want %v", calls, want)
	}
	out[0].Text = "changed"
	if got := m.Candidates()[0].Text; got != "x" {
		t.Errorf("the command line kept the producer's slice: %q", got)
	}
}

// Blurring hides the candidates, and focusing shows them again.
func TestCompleteFollowsFocus(t *testing.T) {
	m := opened(t, "goto cli", WithSize(80, MaxHeight), WithComplete(completeWords(repos...)))
	if m.Height() != 2 {
		t.Fatalf("Height() = %d with candidates, want 2", m.Height())
	}
	m.Blur()
	if m.Height() != 1 || len(m.Candidates()) != 0 {
		t.Errorf("a blurred command line shows %d candidates", len(m.Candidates()))
	}
	m.Focus()
	if m.Height() != 2 {
		t.Errorf("Height() = %d after focus, want 2", m.Height())
	}
	m.SetSize(80, 1)
	if m.Height() != 1 {
		t.Errorf("Height() = %d with room for one row, want 1", m.Height())
	}
}

// The selected candidate stays on the row, however far the user cycles.
func TestCompleteScrolls(t *testing.T) {
	many := make([]string, 0, 26)
	for _, r := range "abcdefghijklmnopqrstuvwxyz" {
		many = append(many, "repo-"+string(r)+"-with-a-long-name")
	}
	m := opened(t, "repo", WithSize(80, MaxHeight), WithComplete(completeWords(many...)))
	for i := range len(many) * 2 {
		m, _ = m.Update(tab)
		end, _ := m.fitRow(m.comp.first)
		if sel := m.Selected(); sel >= 0 && (sel < m.comp.first || sel >= end) {
			t.Fatalf("after %d tabs, candidate %d is off the row [%d, %d)", i+1, sel, m.comp.first, end)
		}
		assertFits(t, m.View(), 80, 2)
	}
}

// Shrinking keeps the selected candidate on the row, and growing fills
// the row again.
func TestCompleteResize(t *testing.T) {
	m := opened(t, "goto repo", WithSize(120, MaxHeight), WithComplete(manyComplete))
	m, _ = press(t, m, shiftTab, shiftTab) // repo-y
	visible := func(w int) {
		t.Helper()
		end, _ := m.fitRow(m.comp.first)
		if sel := m.Selected(); sel < m.comp.first || sel >= end {
			t.Fatalf("at %d columns candidate %d is off the row [%d, %d)", w, sel, m.comp.first, end)
		}
		assertFits(t, m.View(), w, 2)
	}
	for _, w := range []int{40, 20, 60, 120, 260} {
		m.SetSize(w, MaxHeight)
		visible(w)
		// The row is as full as it can be: one more candidate at the
		// front would push the selected one off.
		if f := m.comp.first; f > 0 {
			if end, _ := m.fitRow(f - 1); m.Selected() < end {
				t.Errorf("at %d columns the row starts at %d but could start at %d", w, f, f-1)
			}
		}
	}
	if m.comp.first != 0 {
		t.Errorf("at 260 columns, room for all, the row starts at %d, want 0", m.comp.first)
	}
}

// Help offers tab only with a Complete function.
func TestCompleteHelp(t *testing.T) {
	has := func(m Model, k string) (short, full bool) {
		for _, b := range m.ShortHelp() {
			short = short || b.Help().Key == k
		}
		for _, g := range m.FullHelp() {
			for _, b := range g {
				full = full || b.Help().Key == k
			}
		}
		return short, full
	}
	if s, f := has(New(), "tab"); s || f {
		t.Errorf("without Complete, help offers tab: short %v, full %v", s, f)
	}
	if s, f := has(New(WithComplete(repoComplete)), "tab"); !s || !f {
		t.Errorf("with Complete, help leaves out tab: short %v, full %v", s, f)
	}
}

// A copy of the command line keeps its own candidates. (The text of the
// line is shared between copies that go on editing; see Model.)
func TestCompleteCopies(t *testing.T) {
	a := opened(t, "goto gam", WithSize(80, MaxHeight), WithComplete(repoComplete))
	view := a.View()
	b := a
	b.SetValue("goto charm")
	b, _ = press(t, b, tab)
	if n := len(a.Candidates()); n != 2 || a.Selected() != -1 || a.View() != view {
		t.Errorf("the copy changed a: %d candidates, selected %d", n, a.Selected())
	}
	if n := len(b.Candidates()); n != 3 || b.Selected() != 0 {
		t.Errorf("the copy has %d candidates, selected %d; want 3, 0", n, b.Selected())
	}
}
