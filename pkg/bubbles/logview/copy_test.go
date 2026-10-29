package logview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A copy of the model edits a text of its own: the search prompt edits its
// text in place, so without its own copy, typing into the middle of one
// copy's text would change the other's.
func TestCopyKeepsItsOwnText(t *testing.T) {
	m, _ := keys(t, open(t, WithSize(80, 24)), "/")
	m = typeText(t, m, "abc")
	c := m
	c, _ = keys(t, c, "left", "left")
	c = typeText(t, c, "x")
	if got := m.pg.PromptView(40); !strings.Contains(got, "abc") || strings.Contains(got, "x") {
		t.Errorf("original shows %q after the copy was edited, want abc", got)
	}
	if got := ansi.Strip(c.pg.PromptView(40)); !strings.Contains(got, "axbc") {
		t.Errorf("copy shows %q, want axbc", got)
	}
}
