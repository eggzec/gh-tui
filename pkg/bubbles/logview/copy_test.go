package logview

import (
	"testing"
)

// A copy of the model edits a text of its own: the bubbles text input
// edits its text in place, so without its own copy, typing into the middle
// of one copy's text would change the other's.
func TestCopyKeepsItsOwnText(t *testing.T) {
	m, _ := keys(t, open(t), "/")
	m = typeText(t, m, "abc")
	c := m
	c.input.SetCursor(1)
	c = typeText(t, c, "x")
	if got := m.input.Value(); got != "abc" {
		t.Errorf("original = %q after the copy was edited, want %q", got, "abc")
	}
	if got := c.input.Value(); got != "axbc" {
		t.Errorf("copy = %q, want %q", got, "axbc")
	}
}
