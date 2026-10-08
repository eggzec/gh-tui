package diff

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

// A view made without a key map has no key bound, and still says what each
// does in help.
func TestNoKeyMapBindsNothing(t *testing.T) {
	for _, g := range New((&source{}).fetch).KeyMap().FullHelp() {
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

// Space is bound to nothing until the view can read on.
func TestSpaceIsUnbound(t *testing.T) {
	m := load(t, &source{files: []File{goFile("a.go", 3)}, size: 10})
	before := m.Cursor()
	m = keys(t, m, "space")
	if m.Cursor() != before {
		t.Errorf("space moved the cursor from %d to %d", before, m.Cursor())
	}
	for _, g := range m.FullHelp() {
		for _, b := range g {
			for _, k := range b.Keys() {
				if k == "space" || k == " " {
					t.Errorf("%q is bound to space", b.Help().Desc)
				}
			}
		}
	}
}

// The retry key shows in help only after a fetch failed.
func TestRetryKeyEnabledOnlyOnError(t *testing.T) {
	src := &source{files: []File{goFile("a.go", 1)}, size: 1, fail: errBoom}
	m := load(t, src)
	if !m.KeyMap().Retry.Enabled() {
		t.Error("retry is disabled after a failed fetch")
	}
	src.failWith(nil)
	m = keys(t, m, "r")
	if m.KeyMap().Retry.Enabled() {
		t.Error("retry is enabled after the fetch succeeded")
	}
}
