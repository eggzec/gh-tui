package feed

import "testing"

func TestSelectedRow(t *testing.T) {
	m := load(t, newSource(30, 10), WithSize(40, 8), WithItemHeight(1))
	if got := m.SelectedRow(); got != 0 {
		t.Errorf("SelectedRow at the top = %d, want 0", got)
	}
	m = keys(t, m, "down", "down")
	if got := m.SelectedRow(); got != 2 {
		t.Errorf("SelectedRow after two downs = %d, want 2", got)
	}
	m = keys(t, m, "pgdown", "pgdown", "pgdown")
	if got := m.SelectedRow(); got < 0 || got >= 8 {
		t.Errorf("SelectedRow after scrolling = %d, want a line of the view", got)
	}
}
