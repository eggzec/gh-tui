package pulls

import (
	"testing"
	"time"

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
// own keys before the feed's and the thread's, and the answer alone while
// a change waits for one.
func TestKeyLayersOrder(t *testing.T) {
	h := started(t, newFakeService(), 80, 20)
	for k, want := range map[string]string{
		"]": ui.PullsTitle + ": next state", "r": ui.PullsTitle + ": refresh",
		"m": ui.PullsTitle + ": merge", "j": ui.PullsTitle + ": down",
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
	press(t, h, "[")

	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter opened no pull request")
	}
	for k, want := range map[string]string{
		"esc": "Pull request: back", "r": "Pull request: refresh", "j": "Conversation: down", "]": "nothing",
	} {
		if got := winner(m.KeyLayers(), k); got != want {
			t.Errorf("%s reaches %q in the modal, want %q", k, got, want)
		}
	}
	press(t, h, "m")
	if m.ask == nil {
		t.Fatal("m didn't ask to merge")
	}
	if got := winner(m.KeyLayers(), "esc"); got != "Confirm: no" {
		t.Errorf("esc reaches %q while asking, want the answer", got)
	}
	press(t, h, "esc")
	if m.ask != nil || h.modal() != m {
		t.Error("esc while asking didn't just drop the question")
	}
	press(t, h, "esc")
	if h.modal() != nil {
		t.Error("esc didn't close the modal")
	}
}

// The list moves with the keys of its context, so a user's keys for down
// replace j.
func TestListMovesWithConfiguredKeys(t *testing.T) {
	keys := config.Default().Keys
	keys.Set("pulls.down", []string{"w"})
	s := New(t.Context(), newFakeService(), keys, WithClock(func() time.Time { return clock }))
	s.SetSize(80, 20)
	s.Focus()
	h := &host{Section: s}
	drain(t, h, h.Update(ui.RepoMsg{Repo: repo}))
	drain(t, h, h.Init())

	first, _ := h.target()
	press(t, h, "j")
	if now, _ := h.target(); now.Number != first.Number {
		t.Error("j moved down, but down is bound to w")
	}
	press(t, h, "w")
	if now, _ := h.target(); now.Number == first.Number {
		t.Error("w didn't move down")
	}
}
