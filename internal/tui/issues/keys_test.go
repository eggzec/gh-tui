package issues

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
}

// winner names the binding that k reaches in layers, by its layer.
func winner(layers []keyhelp.Layer, k string) string {
	b, src, ok := uitest.Winner(layers, k)
	if !ok {
		return "nothing"
	}
	return src + ": " + b.Help().Desc
}

// The layers take a key in the order the list and the modal do: their
// own keys before the feed's and the thread's, and the prompt alone while
// it is open.
func TestKeyLayersOrder(t *testing.T) {
	h := started(t, newFakeService(sampleIssues(12)), 80, 20)
	for k, want := range map[string]string{
		"]": ui.IssuesTitle + ": next state", "X": ui.IssuesTitle + ": close issue", "j": ui.IssuesTitle + ": down", "c": "nothing",
	} {
		if got := winner(h.KeyLayers(), k); got != want {
			t.Errorf("%s reaches %q in the list, want %q", k, got, want)
		}
	}
	tab := h.tab
	press(t, h, "]")
	if h.tab == tab {
		t.Error("] didn't switch the state")
	}
	press(t, h, "[", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter opened no issue")
	}
	for k, want := range map[string]string{"esc": "Issue: close", "c": "Issue: comment", "j": "Issue: down"} {
		if got := winner(m.KeyLayers(), k); got != want {
			t.Errorf("%s reaches %q in the modal, want %q", k, got, want)
		}
	}
	press(t, h, "c")
	if m.composing != composeComment {
		t.Fatal("c didn't open the prompt")
	}
	if got := winner(m.KeyLayers(), "j"); got != "nothing" {
		t.Errorf("j reaches %q in the prompt, want it typed", got)
	}
	press(t, h, "esc")
	if m.composing != composeNone || h.modal() != m {
		t.Error("esc in the prompt didn't just close it")
	}
}

// Closing an issue and its labels are capitals, so a stray lowercase key
// does nothing: X asks to close and L opens the labels, and x and l do
// neither.
func TestChangesAreCapitals(t *testing.T) {
	for _, tt := range []struct {
		key  string
		asks bool
	}{
		{"X", true}, {"x", false},
	} {
		h := started(t, newFakeService(sampleIssues(12)), 80, 20)
		press(t, h, "down", tt.key)
		if got := question(h) != ""; got != tt.asks {
			t.Errorf("%s asks %v, want %v", tt.key, got, tt.asks)
		}
	}
	for key, want := range map[string]bool{"L": true, "l": false} {
		svc := newFakeService(sampleIssues(12))
		h, m := opened(t, svc, 30)
		press(t, h, key)
		if got := m.composing == composeLabels; got != want {
			t.Errorf("%s opens the labels prompt: %v, want %v", key, got, want)
		}
	}
}
