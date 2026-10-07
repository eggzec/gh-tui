package filterform

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// The key map shares enter between Edit and Apply, and its picker's keys
// with the form's, so only the form in each state is free of conflicts.
func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, withNormalKeys{DefaultKeyMap()})
}

// withNormalKeys adds the picker's normal-mode keys to full help. The
// form's pickers have no modes, so its help leaves them out.
type withNormalKeys struct{ KeyMap }

func (k withNormalKeys) FullHelp() [][]key.Binding {
	n := k.Picker.Normal
	return append(k.KeyMap.FullHelp(), []key.Binding{
		n.Up, n.Down, n.PageUp, n.PageDown, n.HalfPageUp, n.HalfPageDown,
		n.Top, n.Bottom, n.Insert, n.Append,
	})
}

// Full help enables the keys that act on the row in focus, with no two on
// one key.
func TestFullHelpState(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.Msg
		want []string
	}{
		{name: "choice row", want: []string{"[", "]", "↑", "↑↓", "←", "←→", "space", "↵ apply", "delete", "esc"}},
		{name: "person row", keys: keys(down, rowAuthor), want: []string{"[", "]", "↑", "↑↓", "←", "←→", "space", "↵ edit", "delete", "esc"}},
		{name: "sort row", keys: []tea.Msg{nextTab}, want: []string{"[", "]", "↑", "↑↓", "←", "←→", "space", "↵ apply", "esc"}},
		{name: "text editor", keys: append(keys(down, rowBase), enter), want: []string{"↵ edit", "esc"}},
		{name: "picker", keys: append(keys(down, rowAuthor), enter), want: []string{"space", "↵ edit", "esc", "↑/ctrl+p", "↓/ctrl+n", "pgup", "pgdn"}},
		{name: "query line", keys: keys(down, rowQuery), want: []string{"↑", "↑↓", "↵ apply", "esc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := press(t, open(t, prSpec(nil)), tt.keys...)
			var got []string
			for _, g := range m.FullHelp() {
				for _, b := range g {
					if !b.Enabled() {
						continue
					}
					h := b.Help()
					if h.Key == "↵" {
						h.Key += " " + h.Desc
					}
					got = append(got, h.Key)
				}
			}
			slices.Sort(got)
			want := slices.Sorted(slices.Values(tt.want))
			if !slices.Equal(got, want) {
				t.Errorf("enabled %q, want %q", got, want)
			}
			keytest.NoConflicts(t, m)
		})
	}
}
