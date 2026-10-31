package pulls

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// previewPanes returns a modal zoomed on Merge, whose list has the row
// *row (empty for none) and whose preview is 40 lines long.
func previewPanes(t *testing.T, row *string) *panesModal {
	t.Helper()
	m := sizedPanes(t, 160, 30)
	m.nav.SetFocus(paneMerge)
	m.zoom.SetZoom(true)
	m.slots[paneMerge-1].hovered = func(*panesModal) (string, string, bool) {
		return *row, "row " + *row, *row != ""
	}
	m.slots[paneMerge-1].preview = func(_ *panesModal, _ panes.Box) string {
		lines := make([]string, 40)
		for i := range lines {
			lines[i] = fmt.Sprintf("line %d", i)
		}
		return strings.Join(lines, "\n")
	}
	return m
}

// TestPreviewSubFocus checks that tab moves between the list and the preview
// of a zoomed pane and never to another pane, that a digit focuses the list of
// the pane it names, and that the zoom resets the focus to the list.
func TestPreviewSubFocus(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	m.nav.SetFocus(paneMerge)
	m.zoom.SetZoom(true)
	for i, want := range []panes.Role{panes.Preview, panes.List, panes.Preview, panes.List} {
		key := "tab"
		if i >= 2 {
			key = "shift+tab"
		}
		m.press(keyMsg(key))
		if m.nav.Focus() != paneMerge || m.nav.Sub(paneMerge) != want {
			t.Fatalf("%s %d: focus %v box %v, want Merge and %v", key, i, m.nav.Focus(), m.nav.Sub(paneMerge), want)
		}
	}
	m.press(keyMsg("tab"))
	m.press(keyMsg("4"))
	if m.nav.Focus() != paneInvolved || m.nav.Sub(paneMerge) != panes.List || m.nav.Sub(paneInvolved) != panes.List {
		t.Errorf("4 left focus %v, boxes %v", m.nav.Focus(), m.nav)
	}
	m.press(keyMsg("tab"))
	if m.nav.Sub(paneInvolved) != panes.Preview {
		t.Fatalf("tab left box %v, want Preview", m.nav.Sub(paneInvolved))
	}
	m.press(keyMsg("z"))
	m.press(keyMsg("z"))
	if m.nav.Sub(paneInvolved) != panes.List {
		t.Errorf("unzooming left box %v, want List", m.nav.Sub(paneInvolved))
	}
}

// TestConfirmKeepsPreviewScroll checks that a question opened over a scrolled
// preview leaves the scroll where it was, whatever keys it gets.
func TestConfirmKeepsPreviewScroll(t *testing.T) {
	t.Parallel()
	row := "a"
	m := previewPanes(t, &row)
	m.Update(keyMsg("tab"))
	m.Update(keyMsg("j"))
	m.Update(keyMsg("j"))
	if got := m.offset(); got != 2 {
		t.Fatalf("two j: offset %d, want 2", got)
	}
	m.ask(ui.Confirm{Question: "Close #1?"})
	for _, k := range []string{"j", "k", "G", "ctrl+d"} {
		m.Update(keyMsg(k))
	}
	if got := m.offset(); got != 2 {
		t.Errorf("keys under a question moved the preview to %d, want 2", got)
	}
}

// TestPreviewScroll checks that the keys of the preview scroll it only while
// it has the focus, that each hovered row keeps its own offset, and that a row
// with nothing to preview leaves the last title.
func TestPreviewScroll(t *testing.T) {
	t.Parallel()
	row := "a"
	m := previewPanes(t, &row)
	m.Update(keyMsg("j"))
	if got := m.offset(); got != 0 {
		t.Fatalf("j scrolled the list's preview to %d", got)
	}
	m.Update(keyMsg("tab"))
	for _, k := range []string{"j", "j", "j"} {
		m.Update(keyMsg(k))
	}
	if got := m.offset(); got != 3 {
		t.Fatalf("three j: offset %d, want 3", got)
	}
	for _, k := range []string{"f", "s", "x"} {
		m.Update(keyMsg(k))
	}
	if got := m.offset(); got != 3 {
		t.Errorf("keys that are not the preview's moved it to %d", got)
	}
	m.Update(keyMsg("k"))
	if got := m.offset(); got != 2 {
		t.Errorf("k: offset %d, want 2", got)
	}

	row = "b"
	m.follow()
	if got := m.offset(); got != 0 {
		t.Errorf("a new row starts at offset %d, want 0", got)
	}
	m.Update(keyMsg("G"))
	end := m.offset()
	if end <= 3 {
		t.Fatalf("G scrolled to %d, want the end of the text", end)
	}
	m.Update(keyMsg("G"))
	if m.offset() != end {
		t.Errorf("G again moved to %d, past the end %d", m.offset(), end)
	}
	// The root sends the press of the top action when "g g" completes.
	top := keymap.Actions(m.keys.scroll.Home)
	if len(top) == 0 {
		t.Fatal("the top binding of the preview names no action")
	}
	m.Update(keymap.ActionPress(top[0]))
	if m.offset() != 0 {
		t.Errorf("top: offset %d, want 0", m.offset())
	}
	m.Update(keyMsg("g"))
	if m.offset() != 0 {
		t.Errorf("a lone g moved the preview to %d", m.offset())
	}
	m.Update(keyMsg("G"))

	row = "a"
	m.follow()
	if got := m.offset(); got != 2 {
		t.Errorf("back on the first row: offset %d, want 2", got)
	}
	row = "b"
	m.follow()
	if m.offset() != end {
		t.Errorf("back on the second row: offset %d, want %d", m.offset(), end)
	}

	row = ""
	m.follow()
	if m.hover[paneMerge-1].title != "row b" {
		t.Errorf("a row with nothing to preview left the title %q, want the last one", m.hover[paneMerge-1].title)
	}
	if !strings.Contains(m.View(), "row b") {
		t.Error("the preview doesn't carry the hovered row's text as its title")
	}
}

// TestPreviewKeys checks that the preview lists its keys while it has the
// focus, and the pane's own when the list has it.
func TestPreviewKeys(t *testing.T) {
	t.Parallel()
	row := "a"
	m := previewPanes(t, &row)
	if got := m.KeyLayers()[1].Context; got != "pull_merge" {
		t.Fatalf("with the list focused the layer is %q", got)
	}
	m.Update(keyMsg("tab"))
	l := m.KeyLayers()[1]
	if l.Context != ctxPreview || len(l.Bindings) == 0 {
		t.Errorf("with the preview focused the layer is %q with %d keys", l.Context, len(l.Bindings))
	}
}
