package issues

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// descRows is the height of the Description box of the wide layout.
func descRows(t *testing.T, m *detailModal) int {
	t.Helper()
	b, ok := m.paneLayout().Box(paneDescription, panes.Whole)
	if !ok {
		t.Fatal("no Description box")
	}
	return b.Rect.H
}

func TestDescriptionGrowsAndShrinks(t *testing.T) {
	t.Parallel()
	h, m, d := dividedModal(t, 160, false)
	press(t, h, "1")
	start := descRows(t, m)
	press(t, h, "ctrl+up")
	if got := descRows(t, m); got != start+2 {
		t.Errorf("after ctrl+up Description is %d, want %d", got, start+2)
	}
	press(t, h, "ctrl+down", "ctrl+down")
	if got := descRows(t, m); got != start-2 {
		t.Errorf("after two ctrl+down Description is %d, want %d", got, start-2)
	}
	press(t, h, "=")
	if got := descRows(t, m); got != start || d.Moved() {
		t.Errorf("after reset Description is %d, want %d", got, start)
	}
}

func TestConversationGrowsFromTheDescription(t *testing.T) {
	t.Parallel()
	h, m, _ := dividedModal(t, 160, false)
	press(t, h, "2")
	start := descRows(t, m)
	press(t, h, "ctrl+up")
	if got := descRows(t, m); got != start-2 {
		t.Errorf("after ctrl+up on the Conversation the Description is %d, want %d", got, start-2)
	}
}

func TestDescriptionStopsAtMinimum(t *testing.T) {
	t.Parallel()
	h, m, _ := dividedModal(t, 160, false)
	press(t, h, "1")
	for range 60 {
		press(t, h, "ctrl+down")
	}
	if got := descRows(t, m); got != panes.MinHeight {
		t.Fatalf("Description is %d, want %d", got, panes.MinHeight)
	}
	for range 80 {
		press(t, h, "ctrl+up")
	}
	total := m.paneLayout().Height
	if got := descRows(t, m); got != total-panes.MinHeight {
		t.Fatalf("Description is %d, want %d", got, total-panes.MinHeight)
	}
	press(t, h, "ctrl+down")
	if got := descRows(t, m); got != total-panes.MinHeight-2 {
		t.Errorf("a press at the limit piled up: Description is %d", got)
	}
}

func TestDescriptionSettlesOnResize(t *testing.T) {
	t.Parallel()
	h, m, d := dividedModal(t, 160, false)
	press(t, h, "1")
	for range 80 {
		press(t, h, "ctrl+up")
	}
	m.SetSize(160, 20)
	if got, total := descRows(t, m), m.paneLayout().Height; got != total-panes.MinHeight {
		t.Errorf("Description is %d after resize, want %d", got, total-panes.MinHeight)
	}
	if !d.Desc.Moved() {
		t.Error("the offset was lost")
	}
}

func TestDescriptionKeysDoNothingWithoutADivider(t *testing.T) {
	t.Parallel()
	h, m, d := dividedModal(t, 160, false)
	// The modal starts on Development, in the side column, whose dividers
	// are not the Description's.
	press(t, h, "ctrl+up")
	if d.Desc.Moved() {
		t.Error("the side column moved the stack divider")
	}
	press(t, h, "=", "1", "z")
	press(t, h, "ctrl+up")
	if d.Moved() || m.canStack() {
		t.Error("a zoomed pane moved the stack divider")
	}
	h, m, d = dividedModal(t, 100, false)
	press(t, h, "1")
	press(t, h, "ctrl+up")
	if d.Moved() || m.canStack() {
		t.Error("a narrow modal moved the stack divider")
	}
}

func TestDescriptionDividerSharedByName(t *testing.T) {
	t.Parallel()
	d := &ui.Dividers{Step: 2}
	d.Desc.Move(4)
	def := descriptionHeight(30)
	if got := descSplit(d, 30); got != def+4 {
		t.Errorf("description split %d, want %d", got, def+4)
	}
	if got := descSplit(nil, 30); got != def {
		t.Errorf("nil description split %d, want %d", got, def)
	}
}

func TestDescriptionHelpRowsFollowTheFocus(t *testing.T) {
	t.Parallel()
	h, m, _ := dividedModal(t, 160, false)
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
	if !enabled() {
		t.Error("grow is disabled on the side column")
	}
	press(t, h, "1", "z")
	if enabled() {
		t.Error("grow is enabled on a zoomed pane")
	}
	press(t, h, "z")
	if !enabled() {
		t.Error("grow is disabled on the Description")
	}
}
