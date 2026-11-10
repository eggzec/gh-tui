package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// descHigh is the height of the Description box of the wide layout.
func descHigh(t *testing.T, m *panesModal) int {
	t.Helper()
	b, ok := m.layout().Box(paneDescription, panes.Whole)
	if !ok {
		t.Fatal("no Description box")
	}
	return b.Rect.H
}

func TestDescriptionGrowsAndShrinks(t *testing.T) {
	t.Parallel()
	m, d := dividedPanes(t, 160)
	m.nav.SetFocus(paneDescription)
	start := descHigh(t, m)
	m.press(keyMsg("ctrl+up"))
	if got := descHigh(t, m); got != start+2 {
		t.Errorf("after ctrl+up Description is %d, want %d", got, start+2)
	}
	m.press(keyMsg("ctrl+down"))
	m.press(keyMsg("ctrl+down"))
	if got := descHigh(t, m); got != start-2 {
		t.Errorf("after two ctrl+down Description is %d, want %d", got, start-2)
	}
	m.press(keyMsg("="))
	if got := descHigh(t, m); got != start || d.Moved() {
		t.Errorf("after reset Description is %d, want %d", got, start)
	}
}

func TestConversationGrowsFromTheDescription(t *testing.T) {
	t.Parallel()
	m, _ := dividedPanes(t, 160)
	m.nav.SetFocus(paneConversation)
	start := descHigh(t, m)
	m.press(keyMsg("ctrl+up"))
	if got := descHigh(t, m); got != start-2 {
		t.Errorf("after ctrl+up on the Conversation the Description is %d, want %d", got, start-2)
	}
}

func TestDescriptionStopsAtMinimum(t *testing.T) {
	t.Parallel()
	m, _ := dividedPanes(t, 160)
	m.nav.SetFocus(paneDescription)
	for range 60 {
		m.press(keyMsg("ctrl+down"))
	}
	if got := descHigh(t, m); got != panes.MinHeight {
		t.Fatalf("Description is %d, want %d", got, panes.MinHeight)
	}
	for range 80 {
		m.press(keyMsg("ctrl+up"))
	}
	h := m.areaHeight()
	if got := descHigh(t, m); got != h-panes.MinHeight {
		t.Fatalf("Description is %d, want %d", got, h-panes.MinHeight)
	}
	m.press(keyMsg("ctrl+down"))
	if got := descHigh(t, m); got != h-panes.MinHeight-2 {
		t.Errorf("a press at the limit piled up: Description is %d", got)
	}
}

func TestDescriptionSettlesOnResize(t *testing.T) {
	t.Parallel()
	m, d := dividedPanes(t, 160)
	m.nav.SetFocus(paneDescription)
	for range 80 {
		m.press(keyMsg("ctrl+up"))
	}
	m.SetSize(160, 20)
	if got, h := descHigh(t, m), m.areaHeight(); got != h-panes.MinHeight {
		t.Errorf("Description is %d after resize, want %d", got, h-panes.MinHeight)
	}
	if !d.Desc.Moved() {
		t.Error("the offset was lost")
	}
}

func TestDescriptionKeysDoNothingWithoutADivider(t *testing.T) {
	t.Parallel()
	narrow, d := dividedPanes(t, 100)
	narrow.press(keyMsg("ctrl+up"))
	if d.Moved() || narrow.canStack() {
		t.Error("a narrow modal moved the stack divider")
	}
	m, d := dividedPanes(t, 160)
	m.nav.SetFocus(paneMerge)
	m.press(keyMsg("ctrl+up"))
	if d.Moved() || m.canStack() {
		t.Error("the right column moved the stack divider")
	}
	m.nav.SetFocus(paneDescription)
	m.press(keyMsg("z"))
	m.press(keyMsg("ctrl+up"))
	if d.Moved() || m.canStack() {
		t.Error("a zoomed pane moved the stack divider")
	}
}

func TestDescriptionHelpRowsFollowTheFocus(t *testing.T) {
	t.Parallel()
	m, _ := dividedPanes(t, 160)
	enabled := func() bool {
		for _, l := range m.KeyLayers() {
			for _, b := range l.Bindings {
				if b.Help().Desc == "grow pane" {
					return b.Enabled()
				}
			}
		}
		t.Fatal("no grow row")
		return false
	}
	m.nav.SetFocus(paneDescription)
	if !enabled() {
		t.Error("grow is disabled on the Description")
	}
	m.nav.SetFocus(paneMerge)
	if enabled() {
		t.Error("grow is enabled on the right column")
	}
}
