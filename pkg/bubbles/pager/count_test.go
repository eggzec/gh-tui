package pager

import (
	"strings"
	"testing"
)

func TestCount(t *testing.T) {
	tests := []struct {
		name          string
		keys          []string
		filter        string
		wantTop       int
		wantCapturing bool
		wantClose     bool
	}{
		{name: "a count waits for its key", keys: []string{"4", "2"}, wantTop: 1, wantCapturing: true},
		{name: "g goes to its line", keys: []string{"4", "2", "g"}, wantTop: 42},
		{name: "G goes to its line", keys: []string{"4", "2", "G"}, wantTop: 42},
		{name: "home goes to its line", keys: []string{"7", "home"}, wantTop: 7},
		{name: "a line past the end goes to the last", keys: []string{"9", "9", "9", "g"}, wantTop: 91},
		{name: "zero goes to the first", keys: []string{"G", "0", "g"}, wantTop: 1},
		{name: "the window stops at the end", keys: []string{"9", "5", "g"}, wantTop: 91},
		{name: "g alone goes to the top", keys: []string{"G", "g"}, wantTop: 1},
		{name: "G alone goes to the bottom", keys: []string{"G"}, wantTop: 91},
		{name: "a count lasts one key", keys: []string{"4", "2", "j", "g"}, wantTop: 1},
		{name: "other keys act without it", keys: []string{"4", "2", "j"}, wantTop: 2},
		{name: "esc drops it", keys: []string{"4", "esc", "g"}, wantTop: 1},
		{name: "then closes", keys: []string{"4", "esc", "esc"}, wantTop: 1, wantClose: true},
		{name: "a huge count doesn't overflow", keys: []string{"9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "9", "g"},
			wantTop: 91},
		// Lines 2, 9, 16, … are shown: line 10 is hidden, so 10g goes to
		// line 16.
		{name: "a hidden line goes to the next shown", filter: "line x ", keys: []string{"1", "0", "g"}, wantTop: 16},
		{name: "a shown line goes to it", filter: "line x ", keys: []string{"2", "3", "G"}, wantTop: 23},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
			if tt.filter != "" {
				m, _ = enterAll(t, m, "&", tt.filter, "enter")
				m, _ = keys(t, m, "G")
			}
			m, msg := keys(t, m, tt.keys...)
			if got := m.topLine() + 1; got != tt.wantTop {
				t.Errorf("top line %d, want %d", got, tt.wantTop)
			}
			if m.Capturing() != tt.wantCapturing {
				t.Errorf("capturing %v, want %v", m.Capturing(), tt.wantCapturing)
			}
			if tt.wantCapturing && !strings.HasPrefix(lastLine(plain(m)), "42 ") {
				t.Errorf("status %q doesn't show the count", lastLine(plain(m)))
			}
			if _, ok := msg.(CloseMsg); ok != tt.wantClose {
				t.Errorf("sent %#v, want close %v", msg, tt.wantClose)
			}
		})
	}
}

// New content drops a count or an option the pager waited for, so the
// parent's keys act again.
func TestContentDropsPending(t *testing.T) {
	for _, pending := range [][]string{{"4", "2"}, {"-"}} {
		m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
		m, _ = keys(t, m, pending...)
		if !m.Capturing() {
			t.Fatalf("%v: not capturing", pending)
		}
		m.SetContent("other.txt", numbered(50))
		if m.Capturing() || m.num != 0 || m.KeyMap().Cancel.Enabled() {
			t.Errorf("%v: after new content capturing %v, count %d, cancel enabled %v; want none",
				pending, m.Capturing(), m.num, m.KeyMap().Cancel.Enabled())
		}
		// g goes to the top, not to line 42.
		m, _ = keys(t, m, "G", "g")
		if m.topLine() != 0 {
			t.Errorf("%v: g went to line %d", pending, m.topLine()+1)
		}
	}
}
