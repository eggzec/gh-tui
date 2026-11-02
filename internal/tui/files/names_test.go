package files

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestNodesCleanHostileNames(t *testing.T) {
	t.Parallel()
	hostile := []string{
		"a\x1b]0;owned\x07b",
		"c\x1b[31mred\x1b[0m",
		"bad\xff\xfeutf8",
		"line\nbreak",
	}
	entries := make([]core.TreeEntry, len(hostile))
	for i, name := range hostile {
		entries[i] = core.TreeEntry{Path: "dir/" + name, Name: name, Type: core.EntryBlob}
	}
	s := &source{}
	for i, n := range s.nodes(entries) {
		if strings.ContainsAny(n.Name, "\x1b\x07\n") || strings.Contains(n.Name, "\xff") {
			t.Errorf("name %q of %q reaches the screen raw", n.Name, hostile[i])
		}
	}
}
