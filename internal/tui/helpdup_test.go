package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
)

// TestHelpListsNoRowTwice opens the help in every context of keys and checks
// that no row is listed twice: not a key that works, and not a key that is
// off, such as the back key where there is nothing to go back to.
func TestHelpListsNoRowTwice(t *testing.T) {
	t.Parallel()
	for _, c := range keyContexts() {
		t.Run(strings.NewReplacer(" ", "-", ":", "").Replace(c.name), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				m, _ := c.reach(t)
				driveKeys(t, m, m.openHelp())
				if !m.helpOpen() {
					t.Fatalf("%s: the help didn't open", c.name)
				}
				seen := map[string]int{}
				for _, r := range m.keyhelp.Rows() {
					h := r.Binding.Help()
					if h.Desc == "" || !slices.ContainsFunc(r.Binding.Keys(), func(k string) bool { return slices.Contains(m.keys.Back.Keys(), k) }) {
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
