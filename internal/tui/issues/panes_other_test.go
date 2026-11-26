package issues

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/slots"
)

func TestOtherKeysScrollThePreviewFromTheList(t *testing.T) {
	t.Parallel()
	h, m := openedBlockers(t, &fakePreviews{body: 80})
	m.View()
	key := slots.Key{Pane: paneBlockers, ID: m.slots.Hover(paneBlockers).ID}
	press(t, h, "J", "J")
	if got := m.slots.Previews().Get(key); got != 2 {
		t.Fatalf("offset = %d after two J, want 2", got)
	}
	press(t, h, "K")
	if got := m.slots.Previews().Get(key); got != 1 {
		t.Fatalf("offset = %d after K, want 1", got)
	}
	if m.nav.InPreview(m.paneLayout()) {
		t.Fatal("J moved the focus to the preview")
	}
}

func TestOtherKeysMoveTheListFromThePreview(t *testing.T) {
	t.Parallel()
	h, m := openedBlockers(t, &fakePreviews{body: 80})
	m.View()
	key := slots.Key{Pane: paneBlockers, ID: m.slots.Hover(paneBlockers).ID}
	press(t, h, "tab")
	press(t, h, "j", "j", "j")
	if m.slots.Previews().Get(key) != 3 {
		t.Fatal("the preview didn't scroll")
	}
	press(t, h, "J")
	if got := m.slots.Previews().Get(slots.Key{Pane: paneBlockers, ID: m.slots.Hover(paneBlockers).ID}); got != 0 {
		t.Fatalf("the preview of the row under the cursor starts at %d, want 0", got)
	}
	if !m.nav.InPreview(m.paneLayout()) {
		t.Fatal("J left the preview")
	}
}
