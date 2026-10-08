package feed

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	km := testKeyMap
	keytest.Complete(t, km)
	keytest.Tagged(t, km)
	keytest.HelpTags(t, km)
	keytest.NoConflicts(t, km)
}

// testKeys are the keys the tests of the feed bind, as an app would.
var testKeys = map[string][]string{
	"up":             {"up", "k"},
	"down":           {"down", "j"},
	"page_up":        {"ctrl+b", "pgup"},
	"page_down":      {"ctrl+f", "pgdown"},
	"half_page_up":   {"ctrl+u"},
	"half_page_down": {"ctrl+d"},
	"top":            {"g", "home"},
	"bottom":         {"G", "end"},
	"global.refresh": {"r"},
}

var testKeyMap = NewKeyMap(keytest.Table(testKeys))

// newModel makes a feed with the keys of a pane, as the app does.
func newModel[T any](fetch Fetch[T], render Render[T], opts ...Option) Model[T] {
	return New(fetch, render, append([]Option{WithKeyMap(testKeyMap)}, opts...)...)
}

// A feed made without a key map has no key bound, and still says what
// each does in help.
func TestNoKeyMapBindsNothing(t *testing.T) {
	for _, g := range New(newSource(1, 1).fetch, renderItem).KeyMap().FullHelp() {
		for _, b := range g {
			if b.Enabled() || len(b.Keys()) != 0 {
				t.Errorf("%q is bound to %v by default", b.Help().Desc, b.Keys())
			}
			if b.Help().Desc == "" {
				t.Error("a binding lost its help text")
			}
		}
	}
}
