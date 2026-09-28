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
		"]": ui.IssuesTitle + ": next state", "x": ui.IssuesTitle + ": close", "j": "list: down", "c": "nothing",
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
	for k, want := range map[string]string{"esc": "issue: back", "c": "issue: comment", "j": "thread: down"} {
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
