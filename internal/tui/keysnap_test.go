package tui

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"text/tabwriter"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// keySnapshot lists every binding of layers, a layer at a time in the
// order they match keys: its keys, the key and label the help shows,
// and whether it is on or off.
func keySnapshot(layers []keyhelp.Layer) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for i, l := range layers {
		if i > 0 {
			fmt.Fprintln(w)
		}
		name := l.Source
		if l.Typing {
			name += " (types)"
		}
		fmt.Fprintln(w, name)
		for _, k := range l.Bindings {
			state := "on"
			if !k.Enabled() {
				state = "off"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", strings.Join(k.Keys(), " "), k.Help().Key, k.Help().Desc, state)
		}
	}
	w.Flush()
	return b.String()
}

// TestKeySnapshots checks every binding of each context against its
// golden file, so that a change of a key or its label shows as a diff
// that reads. Refresh them with -update.
func TestKeySnapshots(t *testing.T) {
	seen := map[string]string{}
	for _, c := range keyContexts() {
		file := contextFile(c.name)
		if other, ok := seen[file]; ok {
			t.Fatalf("%q and %q have the same file %s", other, c.name, file)
		}
		seen[file] = c.name
		t.Run(file, func(t *testing.T) {
			var layers []keyhelp.Layer
			synctest.Test(t, func(t *testing.T) { layers = c.layers(t) })
			golden.RequireEqual(t, keySnapshot(layers))
		})
	}
}
