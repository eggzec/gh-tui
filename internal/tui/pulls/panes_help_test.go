package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ ui.HelpHost = (*panesModal)(nil)

// TestHelpOverlay checks that the help opens over the panes as an overlay that
// has the keys, and that esc closes it alone.
func TestHelpOverlay(t *testing.T) {
	t.Parallel()
	m := sizedPanes(t, 160, 40)
	if _, ok := m.OpenHelp(m.KeyLayers(), "Help"); !ok || m.top() == nil {
		t.Fatalf("OpenHelp = ok %v, overlay open %v", ok, m.top() != nil)
	}
	if got := m.KeyLayers(); len(got) != 1 || got[0].Context != "help" {
		t.Fatalf("layers with the help open: %d, want the help's alone", len(got))
	}
	if cmd := m.Update(keyMsg("esc")); cmd != nil {
		m.Update(cmd())
	}
	if m.top() != nil {
		t.Error("esc left the help open")
	}
	if m.closed {
		t.Error("esc closed the modal with the help")
	}
}

// TestHelpOverlayBackspace checks that backspace closes the help when it has no
// query, as it closes any viewer, and deletes a character of the query first
// when it has one.
func TestHelpOverlayBackspace(t *testing.T) {
	t.Parallel()
	m := sizedPanes(t, 160, 40)
	m.OpenHelp(m.KeyLayers(), "Help")
	m.Update(keyMsg("backspace"))
	if m.top() != nil || m.closed {
		t.Fatalf("backspace on an empty query: help open %v, modal closed %v", m.top() != nil, m.closed)
	}

	m.OpenHelp(m.KeyLayers(), "Help")
	m.Update(keyMsg("a"))
	m.Update(keyMsg("backspace"))
	h, ok := m.top().(*helpOverlay)
	if !ok || h.h.Query() != "" {
		t.Fatalf("backspace with a query: help open %v", ok)
	}
	m.Update(keyMsg("backspace"))
	if m.top() != nil {
		t.Error("the second backspace left the help open")
	}
}
