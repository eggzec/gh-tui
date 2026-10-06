package notifications

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
}

// The layers take a key in the order the section does: its own keys
// before the list's.
func TestKeyLayersOrder(t *testing.T) {
	s := newSection(t, newFake(inbox()...), 100, 10)
	for k, want := range map[string]string{"m": ui.NotificationsTitle + ": read", "r": ui.NotificationsTitle + ": refresh", "j": ui.NotificationsTitle + ": down"} {
		b, src, _ := uitest.Winner(s.KeyLayers(), k)
		if got := src + ": " + b.Help().Desc; got != want {
			t.Errorf("%s reaches %q, want %q", k, got, want)
		}
	}
	before, _ := s.feed.Selected()
	press(t, s, "j")
	if after, _ := s.feed.Selected(); after.ID == before.ID {
		t.Error("j didn't move down the list")
	}
	if msgs := press(t, s, "m"); len(msgs) == 0 {
		t.Error("m didn't ask to mark the thread read")
	}
}
