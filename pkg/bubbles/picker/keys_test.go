package picker

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, DefaultKeyMap())
	keytest.NoConflicts(t, DefaultKeyMap())
}

// Full help is the same set of bindings in every state, with the keys of
// the mode the picker is in enabled and no two on one key.
func TestFullHelpByMode(t *testing.T) {
	enabled := func(m Model) []string {
		var got []string
		for _, g := range m.FullHelp() {
			for _, b := range g {
				if b.Enabled() {
					got = append(got, b.Help().Key)
				}
			}
		}
		slices.Sort(got)
		return got
	}
	typing := []string{"enter", "esc", "↑/ctrl+p", "↓/ctrl+n", "pgdn", "pgup"}
	normal := []string{"G/end", "a", "ctrl+b/pgup", "ctrl+d", "ctrl+f/pgdn", "ctrl+u", "enter", "esc", "g/home", "i", "j/↓", "k/↑"}
	tests := []struct {
		name string
		opts []Option
		keys []tea.Msg
		want []string
	}{
		{"without modes", nil, nil, typing},
		{"normal mode", []Option{WithModes(true)}, nil, normal},
		{"insert mode", []Option{WithModes(true)}, []tea.Msg{tea.KeyPressMsg{Code: 'i', Text: "i"}}, typing},
		{"normal mode without a filter line", []Option{WithModes(true), WithFilterLine(false)}, nil, slices.DeleteFunc(slices.Clone(normal), func(k string) bool { return k == "i" || k == "a" })},
		{"scopes in insert mode", []Option{WithModes(true), WithScopes("A")}, []tea.Msg{tea.KeyPressMsg{Code: 'i', Text: "i"}}, append(slices.Clone(typing), "shift+tab", "tab")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, nil, append([]Option{WithItems(catalog)}, tt.opts...)...)
			m, _ = press(t, m, tt.keys...)
			if got, want := enabled(m), slices.Sorted(slices.Values(tt.want)); !slices.Equal(got, want) {
				t.Errorf("enabled %q, want %q", got, want)
			}
			keytest.NoConflicts(t, m)
		})
	}
}
