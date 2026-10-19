package keymap_test

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// TestLabelsOfSequences checks that a sequence of one-character keys is
// labelled without a space, and any other with one.
func TestLabelsOfSequences(t *testing.T) {
	for _, tt := range []struct {
		keys []string
		want string
	}{
		{[]string{"g g"}, "gg"},
		{[]string{"g", "g g"}, "g/gg"},
		{[]string{"ctrl+w x"}, "^w x"},
		{[]string{"g enter"}, "g ↵"},
		{[]string{"space g"}, "space g"},
		{[]string{"G g", "home"}, "Gg/home"},
	} {
		if got := keymap.Labels(tt.keys); got != tt.want {
			t.Errorf("Labels(%q) = %q, want %q", tt.keys, got, tt.want)
		}
	}
}

// TestSequences checks that the sequences of a binding are found, and that
// a name of anything but two keys is none.
func TestSequences(t *testing.T) {
	b := key.NewBinding(key.WithKeys("g", "g g", "ctrl+w x", "a b c", "g  "))
	want := [][2]string{{"g", "g"}, {"ctrl+w", "x"}}
	if got := keymap.Sequences(b); !slices.Equal(got, want) {
		t.Errorf("Sequences = %q, want %q", got, want)
	}
	if got := keymap.Sequences(key.NewBinding(key.WithKeys("g"))); got != nil {
		t.Errorf("Sequences of a single key = %q, want none", got)
	}
}
