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
	press(t, h, "J")
	m.View()
	press(t, h, "J")
	m.View()
	if got := m.dev.zoom.Top(); got != 2 {
		t.Fatalf("offset = %d after two J from the list, want 2", got)
	}
	press(t, h, "K")
	m.View()
	if got := m.dev.zoom.Top(); got != 1 {
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
	m.View()
	press(t, h, "down")
	m.View()
	if m.dev.zoom.Top() == 0 {
		t.Fatal("the preview didn't scroll")
	}
	press(t, h, "J")
	next := m.slots.Hover(paneDevelopment).ID
	if next == first {
		t.Fatalf("J from the preview left the list on %q", first)
	}
	m.View()
	if got := m.dev.zoom.Top(); got != 0 {
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

func TestZoomedPreviewRouting(t *testing.T) {
	t.Parallel()
	h := openedDev(t, &fakeDev{links: devLinks(), body: 80})
	m := h.modal()
	press(t, h, "z")
	press(t, h, "down")
	m.View()
	press(t, h, "tab")
	m.View()
	z := m.dev.zoom
	if !m.ctl.Nav.InPreview(m.layout()) || !z.Pager().Focused() {
		t.Fatal("tab didn't give the pager the keys")
	}
	if !z.Wants(keyMsg("j")) || z.Wants(keyMsg("tab")) {
		t.Error("Wants is wrong for j or tab")
	}
	helps := func() (pager, capturing bool) {
		for _, l := range m.KeyLayers() {
			capturing = capturing || l.Context == "search_prompt"
			for _, b := range l.Bindings {
				pager = pager || b.Help().Desc == "down" && b.Enabled()
			}
		}
		return pager, capturing
	}
	if pager, capturing := helps(); !pager || capturing {
		t.Errorf("help in the preview: pager keys %v, prompt %v", pager, capturing)
	}
	press(t, h, "/")
	if !z.Capturing() {
		t.Fatal("/ didn't open the search of the pager")
	}
	if _, capturing := helps(); !capturing {
		t.Error("help doesn't show the search prompt while it is open")
	}
	// A key that would flip the split goes to the prompt instead.
	press(t, h, "tab")
	if !m.ctl.Nav.InPreview(m.layout()) {
		t.Error("tab left the preview while the prompt was open")
	}
	press(t, h, "esc")
	if z.Capturing() || h.modal() == nil {
		t.Fatal("esc didn't close just the prompt")
	}
	press(t, h, "tab")
	m.View()
	if m.ctl.Nav.InPreview(m.layout()) || z.Pager().Focused() {
		t.Error("tab didn't flip back to the list")
	}
}
