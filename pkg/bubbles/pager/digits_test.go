package pager

import "testing"

// Digits are the pane keys of the screens that show a pager, so the pager
// ignores them: g and G always go to the top and bottom.
func TestDigitsAreIgnored(t *testing.T) {
	tests := []struct {
		name    string
		keys    []string
		wantTop int
	}{
		{name: "a digit does nothing", keys: []string{"4", "2"}, wantTop: 1},
		{name: "a digit before G goes to the bottom", keys: []string{"5", "G"}, wantTop: 91},
		{name: "digits before g go to the top", keys: []string{"G", "4", "2", "g"}, wantTop: 1},
		{name: "a digit before home goes to the top", keys: []string{"G", "7", "home"}, wantTop: 1},
		{name: "% does nothing", keys: []string{"5", "0", "%"}, wantTop: 1},
		{name: "other keys act as they do", keys: []string{"4", "2", "j"}, wantTop: 2},
		{name: "a digit doesn't capture the keys", keys: []string{"4", "esc"}, wantTop: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
			m, _ = keys(t, m, tt.keys...)
			if got := m.topLine() + 1; got != tt.wantTop {
				t.Errorf("top line %d, want %d", got, tt.wantTop)
			}
			if m.Capturing() {
				t.Error("capturing")
			}
		})
	}
}

// New content drops an option the pager waited for, so the parent's keys
// act again.
func TestContentDropsPending(t *testing.T) {
	m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
	m, _ = keys(t, m, "-")
	if !m.Capturing() {
		t.Fatal("not capturing")
	}
	m.SetContent("other.txt", numbered(50))
	if m.Capturing() || m.KeyMap().Cancel.Enabled() {
		t.Errorf("after new content capturing %v, cancel enabled %v; want none", m.Capturing(), m.KeyMap().Cancel.Enabled())
	}
}
