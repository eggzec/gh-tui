package cmdline

import (
	"slices"
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestHistory(t *testing.T) {
	history := []string{"goto cli/cli", "search is:open", "goto gammons/slk", "theme dark"}
	tests := []struct {
		name    string
		history []string
		initial string
		keys    []tea.Msg
		want    string
	}{
		{name: "up recalls the last line", keys: []tea.Msg{up}, want: "theme dark"},
		{name: "up again goes further back", keys: []tea.Msg{up, up}, want: "goto gammons/slk"},
		{name: "ctrl+p is up", keys: []tea.Msg{tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}}, want: "theme dark"},
		{name: "up stops at the oldest", keys: []tea.Msg{up, up, up, up, up, up}, want: "goto cli/cli"},
		{name: "what was typed filters", initial: "goto", keys: []tea.Msg{up}, want: "goto gammons/slk"},
		{name: "the filter holds on the walk", initial: "goto", keys: []tea.Msg{up, up}, want: "goto cli/cli"},
		{name: "the filter stops at the oldest match", initial: "goto", keys: []tea.Msg{up, up, up}, want: "goto cli/cli"},
		{name: "down comes back", initial: "goto", keys: []tea.Msg{up, up, down}, want: "goto gammons/slk"},
		{name: "down past the newest restores the typed line", initial: "goto", keys: []tea.Msg{up, down}, want: "goto"},
		{name: "down without a walk does nothing", initial: "go", keys: []tea.Msg{down}, want: "go"},
		{name: "no match stays put", initial: "zzz", keys: []tea.Msg{up}, want: "zzz"},
		{
			name: "editing ends the walk", initial: "goto",
			keys: []tea.Msg{up, up, bksp, bksp, bksp, up},
			// The walk starts again from "goto cli/" and the newest end.
			want: "goto cli/cli",
		},
		{
			name: "moving the cursor keeps the walk", initial: "goto",
			keys: []tea.Msg{up, left, up},
			want: "goto cli/cli",
		},
		{
			name: "lines like the one shown are skipped", history: []string{"a", "b", "b"},
			keys: []tea.Msg{up, up}, want: "a",
		},
		{name: "no history", history: []string{}, keys: []tea.Msg{up, down}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := history
			if tt.history != nil {
				h = tt.history
			}
			m := opened(t, tt.initial, WithSize(80, MaxHeight), WithHistory(h))
			m, _ = press(t, m, tt.keys...)
			if v := m.Value(); v != tt.want {
				t.Errorf("Value() = %q, want %q", v, tt.want)
			}
			if p, n := m.input.Position(), len([]rune(m.Value())); p != n && !slices.Contains(tt.keys, tea.Msg(left)) {
				t.Errorf("cursor at %d, want the end, %d", p, n)
			}
		})
	}
}

// The command line adds what is submitted to its history, once, and the
// next command recalls it.
func TestHistoryRemembers(t *testing.T) {
	m := opened(t, "", WithSize(80, MaxHeight), WithHistory([]string{"goto cli/cli", "theme dark"}))
	for _, line := range []string{" search bugs ", "goto cli/cli"} {
		m.Open(line)
		m, _ = press(t, m, enter)
	}
	want := []string{"theme dark", "search bugs", "goto cli/cli"}
	if h := m.History(); !slices.Equal(h, want) {
		t.Errorf("History() = %q, want %q", h, want)
	}
	m.Open("")
	m, _ = press(t, m, up, up)
	if m.Value() != "search bugs" {
		t.Errorf("up, up recalled %q, want %q", m.Value(), "search bugs")
	}
	// A cancelled line isn't remembered.
	m.Open("quit")
	m, _ = press(t, m, esc)
	if h := m.History(); !slices.Equal(h, want) {
		t.Errorf("History() after cancel = %q, want %q", h, want)
	}
}

// The command line keeps its own copy of the history, and copies of the
// model don't share theirs.
func TestHistoryCopies(t *testing.T) {
	lines := []string{"a", "b"}
	m := opened(t, "", WithSize(80, MaxHeight), WithHistory(lines))
	lines[1] = "changed"
	m.SetHistory(lines[:1:2])
	if h := m.History(); !slices.Equal(h, []string{"a"}) {
		t.Fatalf("History() = %q, want [a]", h)
	}
	a := m
	a.Open("x")
	a, _ = press(t, a, enter)
	if h := m.History(); !slices.Equal(h, []string{"a"}) {
		t.Errorf("a copy's submit changed the history to %q", h)
	}
	if h := a.History(); !slices.Equal(h, []string{"a", "x"}) {
		t.Errorf("the copy's History() = %q, want [a x]", h)
	}
	a.History()[0] = "changed"
	if a.History()[0] != "a" {
		t.Error("History() returned the command line's own slice")
	}
}

// A recalled line gets its candidates like a typed one.
func TestHistoryCompletes(t *testing.T) {
	m := opened(t, "", WithSize(80, MaxHeight),
		WithHistory([]string{"goto gammons/sl"}), WithComplete(repoComplete))
	m, _ = press(t, m, up)
	if n := len(m.Candidates()); n != 2 || m.Height() != 2 {
		t.Errorf("%d candidates and %d rows after up, want 2 and 2", n, m.Height())
	}
	// Inserting a candidate ends the walk, so down has nothing to go back to.
	m, _ = press(t, m, tab, down)
	if m.Value() != "goto gammons/slk" {
		t.Errorf("down after tab left %q, want %q", m.Value(), "goto gammons/slk")
	}
}

// Coming back down from a walk puts the cursor back where it was.
func TestHistoryRestoresCursor(t *testing.T) {
	m := opened(t, "goto", WithSize(80, MaxHeight), WithHistory([]string{"goto cli/cli"}))
	m, _ = press(t, m, left, left, up)
	if m.Value() != "goto cli/cli" {
		t.Fatalf("up recalled %q", m.Value())
	}
	m, _ = press(t, m, down)
	if m.Value() != "goto" || m.input.Position() != 2 {
		t.Errorf("down left %q with the cursor at %d, want %q at 2", m.Value(), m.input.Position(), "goto")
	}
}

func TestHistoryLimit(t *testing.T) {
	lines := func(n int) []string {
		out := make([]string, 0, n)
		for i := range n {
			out = append(out, "goto repo-"+strconv.Itoa(i))
		}
		return out
	}
	if h := New(WithHistory(lines(150))).History(); len(h) != DefaultHistoryLimit || h[0] != "goto repo-50" {
		t.Errorf("by default the history keeps %d lines from %q, want %d from goto repo-50",
			len(h), h[0], DefaultHistoryLimit)
	}
	if h := New(WithHistory(lines(150)), WithHistoryLimit(0)).History(); len(h) != 150 {
		t.Errorf("without a limit the history keeps %d lines, want 150", len(h))
	}

	m := opened(t, "", WithSize(80, MaxHeight), WithHistoryLimit(3))
	m.SetHistory(lines(5))
	want := []string{"goto repo-2", "goto repo-3", "goto repo-4"}
	if h := m.History(); !slices.Equal(h, want) {
		t.Errorf("SetHistory kept %q, want %q", h, want)
	}
	m.Open("quit")
	m, _ = press(t, m, enter)
	want = []string{"goto repo-3", "goto repo-4", "quit"}
	if h := m.History(); !slices.Equal(h, want) {
		t.Errorf("after a submit the history is %q, want %q", h, want)
	}
}

// Help offers up and down only with a history.
func TestHistoryHelp(t *testing.T) {
	has := func(m Model) bool {
		for _, g := range m.FullHelp() {
			for _, b := range g {
				if b.Enabled() && b.Help().Key == "↑" {
					return true
				}
			}
		}
		return false
	}
	if has(New()) {
		t.Error("without a history, help offers up")
	}
	if !has(New(WithHistory([]string{"quit"}))) {
		t.Error("with a history, help leaves out up")
	}
}
