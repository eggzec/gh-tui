package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// TestPullModalHelp checks that the help key in the modal of a pull request
// opens the modal's own help over its panes and not the app's, and that the
// keys it lists are those of the focused pane, the modal and the app.
func TestPullModalHelp(t *testing.T) {
	t.Parallel()
	for _, c := range keyContexts() {
		if c.name != "pull request: zoomed" && c.name != "pull request: files" {
			continue
		}
		t.Run(strings.NewReplacer(" ", "-", ":", "").Replace(c.name), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				m, layers := c.reach(t)
				have := map[string]bool{}
				for _, l := range layers {
					have[l.Context] = true
				}
				for _, want := range []string{c.context, "pull_modal", "global"} {
					if !have[want] {
						t.Errorf("the keys before the help lack the context %s: %v", want, have)
					}
				}
				press, ok := keyPress(config.Default().Keys.Of("global.help")[0])
				if !ok {
					t.Fatal("the help key is no key press")
				}
				driveKeys(t, m, m.key(press))
				if m.helpOpen() {
					t.Error("the app's help opened over the modal")
				}
				if !slices.ContainsFunc(m.keyLayers(), func(l keyhelp.Layer) bool { return l.Context == "help" }) {
					t.Errorf("the modal's help isn't open: layers %v", layerNames(m.keyLayers()))
				}
			})
		})
	}
}

// TestHelpListsNoRowTwice opens the help in every context of keys and checks
// that no row, with the same keys and label, is listed twice: not a key that
// works, and not a key that is off, such as the back key where there is
// nothing to go back to.
func TestHelpListsNoRowTwice(t *testing.T) {
	t.Parallel()
	for _, c := range keyContexts() {
		t.Run(strings.NewReplacer(" ", "-", ":", "").Replace(c.name), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				m, _ := c.reach(t)
				layers := ui.NameLayerActions(m.keyLayers())
				driveKeys(t, m, m.openHelp())
				help := m.keyhelp
				if !m.helpOpen() {
					// A modal with overlays shows the help inside itself; its
					// rows are those of the same layers.
					if !slices.ContainsFunc(m.keyLayers(), func(l keyhelp.Layer) bool { return l.Context == "help" }) {
						t.Fatalf("%s: the help didn't open", c.name)
					}
					help = keyhelp.New()
					help.SetLayers(layers)
				}
				seen := map[string]int{}
				for _, r := range help.Rows() {
					h := r.Binding.Help()
					if h.Desc == "" {
						continue
					}
					id := fmt.Sprintf("%v %q %q", r.Binding.Keys(), h.Key, h.Desc)
					if first, ok := seen[id]; ok {
						t.Errorf("%s: the help lists %s twice (layers %d and %d, %s, %s)", c.name, id, first, r.Layer, r.Source, r.Status)
					}
					seen[id] = r.Layer
				}
			})
		})
	}
}
