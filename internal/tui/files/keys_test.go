package files

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
	keytest.Complete(t, newFinderKeys(config.Default().Keys))
}

// winner names the binding that k reaches in layers, by its layer.
func winner(layers []keyhelp.Layer, k string) string {
	b, src, ok := uitest.Winner(layers, k)
	if !ok {
		return "nothing"
	}
	return src + ": " + b.Help().Desc
}

// The layers take a key in the order the section, the preview and the
// finder do: their own keys first, and a query types the rest.
func TestKeyLayersOrder(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12))
	for k, want := range map[string]string{"r": ui.FilesTitle + ": refresh", "enter": ui.FilesTitle + ": preview", "j": ui.FilesTitle + ": down"} {
		if got := winner(h.s.KeyLayers(), k); got != want {
			t.Errorf("%s reaches %q in the tree, want %q", k, got, want)
		}
	}

	h = openRow(t, sampleFake(), rowAgents)
	p, ok := h.top().(*preview)
	if !ok {
		t.Fatalf("enter opened %T, want a preview", h.top())
	}
	for k, want := range map[string]string{"o": "File: open", "/": "File: search"} {
		if got := winner(p.KeyLayers(), k); got != want {
			t.Errorf("%s reaches %q in the preview, want %q", k, got, want)
		}
	}
	h.keys("/")
	if got := winner(p.KeyLayers(), "o"); got != "nothing" {
		t.Errorf("o reaches %q while searching, want it typed", got)
	}

	h = newHost(loaded(t, sampleFake(), 120, 30))
	f := findIn(t, h)
	for k, want := range map[string]string{"ctrl+t": "Finder: open in tree", "j": "nothing", "down": "Finder: down"} {
		if got := winner(f.KeyLayers(), k); got != want {
			t.Errorf("%s reaches %q in the finder, want %q", k, got, want)
		}
	}
}
