package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/slots"
)

func TestOtherKeysFromTheList(t *testing.T) {
	t.Parallel()
	h := openedDev(t, &fakeDev{links: devLinks(), body: 80})
	m := h.modal()
	press(t, h, "z")
	press(t, h, "down")
	m.View()
	key := slots.Key{Pane: paneDevelopment, ID: m.slots.Hover(paneDevelopment).ID}
	press(t, h, "J")
	press(t, h, "J")
	if got := m.slots.Previews().Get(key); got != 2 {
		t.Fatalf("offset = %d after two J from the list, want 2", got)
	}
	press(t, h, "K")
	if got := m.slots.Previews().Get(key); got != 1 {
		t.Fatalf("offset = %d after K, want 1", got)
	}
	if m.ctl.Nav.InPreview(m.layout()) {
		t.Fatal("J moved the focus to the preview")
	}
}

func TestOtherKeysFromThePreview(t *testing.T) {
	t.Parallel()
	h := openedDev(t, &fakeDev{links: devLinks(), body: 80})
	m := h.modal()
	press(t, h, "z")
	press(t, h, "down")
	m.View()
	first := m.slots.Hover(paneDevelopment).ID
	press(t, h, "tab")
	press(t, h, "down")
	press(t, h, "down")
	if m.slots.Previews().Get(slots.Key{Pane: paneDevelopment, ID: first}) == 0 {
		t.Fatal("the preview didn't scroll")
	}
	press(t, h, "J")
	next := m.slots.Hover(paneDevelopment).ID
	if next == first {
		t.Fatalf("J from the preview left the list on %q", first)
	}
	if got := m.slots.Previews().Get(slots.Key{Pane: paneDevelopment, ID: next}); got != 0 {
		t.Fatalf("the preview of the new row starts at %d, want 0", got)
	}
	if !m.ctl.Nav.InPreview(m.layout()) {
		t.Fatal("J left the preview")
	}
	press(t, h, "K")
	if got := m.slots.Hover(paneDevelopment).ID; got != first {
		t.Fatalf("K from the preview hovers %q, want %q", got, first)
	}
}

func TestOtherKeysUnzoomedDoNothing(t *testing.T) {
	t.Parallel()
	h := openedDev(t, &fakeDev{links: devLinks(), body: 80})
	m := h.modal()
	press(t, h, "down")
	m.View()
	id := m.slots.Hover(paneDevelopment).ID
	press(t, h, "J")
	if got := m.slots.Previews().Get(slots.Key{Pane: paneDevelopment, ID: id}); got != 0 {
		t.Fatalf("J scrolled a preview that isn't shown: %d", got)
	}
	if m.slots.Hover(paneDevelopment).ID != id {
		t.Fatal("J moved the list while unzoomed")
	}
}
