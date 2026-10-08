package graph

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

// testKeys are the keys the tests of the graph bind, as an app would.
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
	"global.select":  {"enter"},
}

var testKeyMap = NewKeyMap(keytest.Table(testKeys))

// newModel makes a graph with the keys of a pane, as the app does.
func newModel(fetch Fetch, opts ...Option) Model {
	return New(fetch, append([]Option{WithKeyMap(testKeyMap)}, opts...)...)
}

// A graph made without a key map has no key bound, and still says what
// each does in help.
func TestNoKeyMapBindsNothing(t *testing.T) {
	for _, g := range New(newSource(sample(), 10).fetch).KeyMap().FullHelp() {
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
