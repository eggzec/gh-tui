package history

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(testKeys()))
}

// The layers take a key in the order the modal does: its own keys before
// the focused pane's, the pager's back in a patch, and a search of the
// patch alone while it is open.
func TestKeyLayersOrder(t *testing.T) {
	m, h := newModal(t, newFake(), 108, 30)
	winner := func(k string) string {
		b, src, ok := uitest.Winner(m.KeyLayers(), k)
		if !ok {
			return "nothing"
		}
		return src + ": " + b.Help().Desc
	}
	steps := []struct {
		keys []string
		want map[string]string
	}{
		{nil, map[string]string{"enter": "graph: diff", "esc": "history: back", "tab": "history: pane"}},
		{[]string{"esc"}, map[string]string{"enter": "history: graph", "/": "history: filter", "esc": "history: close", "j": "branches: down"}},
		{[]string{"enter", "enter"}, map[string]string{"enter": "history: patch", "j": "files: down"}},
		{[]string{"j", "enter"}, map[string]string{"esc": "pager: close", "j": "pager: down", "tab": "history: pane"}},
		{[]string{"/"}, map[string]string{"j": "nothing", "esc": "pager: cancel"}},
	}
	for _, s := range steps {
		h.keys(s.keys...)
		for k, want := range s.want {
			if got := winner(k); got != want {
				t.Errorf("after %q, %s reaches %q, want %q", s.keys, k, got, want)
			}
		}
	}
	h.keys("esc", "esc")
	if m.focus != commitPane || m.commit.patch {
		t.Errorf("esc, esc left focus %d, patch %v; want the files", m.focus, m.commit.patch)
	}
}
