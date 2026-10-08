package calendar

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

// testKeys are the keys the tests of the calendar bind, as an app would.
var testKeys = map[string][]string{
	"up":     {"up", "k"},
	"down":   {"down", "j"},
	"left":   {"left", "h"},
	"right":  {"right", "l"},
	"top":    {"g", "home"},
	"bottom": {"G", "end"},
}

var testKeyMap = NewKeyMap(keytest.Table(testKeys))

// newModel makes a calendar with the default keys of a pane, as the app
// does.
func newModel(opts ...Option) Model {
	return New(append([]Option{WithKeyMap(testKeyMap)}, opts...)...)
}

// A calendar made without a key map has no key bound, and still says what
// each does in help.
func TestNoKeyMapBindsNothing(t *testing.T) {
	for _, g := range New().KeyMap().FullHelp() {
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
