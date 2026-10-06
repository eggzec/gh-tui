package tui

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// snapshotHeader names the fields of a binding's line in a snapshot.
const snapshotHeader = "# label | keys | help key | on or off"

// keySnapshot lists every binding of layers, a layer at a time in the
// order they match keys: its label, its keys, the key the help shows,
// and whether it is on. The fields are not aligned, and the label comes
// first, so that a change of a key changes only the lines of its
// bindings.
func keySnapshot(layers []keyhelp.Layer) string {
	var b strings.Builder
	b.WriteString(snapshotHeader + "\n")
	for _, l := range layers {
		name := l.Source
		if l.Typing {
			name += " (types)"
		}
		fmt.Fprintf(&b, "\n%s\n", name)
		for _, k := range l.Bindings {
			state := "on"
			if !k.Enabled() {
				state = "off"
			}
			fmt.Fprintf(&b, "  %s | %s | %s | %s\n", k.Help().Desc, strings.Join(k.Keys(), " "), k.Help().Key, state)
		}
	}
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
