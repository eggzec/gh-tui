package picker

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, withNormalKeys{DefaultKeyMap()})
	keytest.Tagged(t, DefaultKeyMap())
	keytest.HelpTags(t, DefaultKeyMap())
	keytest.NoConflicts(t, DefaultKeyMap())
}

// withNormalKeys adds the normal-mode keys to the full help of the key map,
// which lists those of a picker without modes only.
type withNormalKeys struct{ KeyMap }

func (k withNormalKeys) FullHelp() [][]key.Binding {
	n := k.Normal
	return append(k.KeyMap.FullHelp(), []key.Binding{
		n.Up, n.Down, n.PageUp, n.PageDown, n.HalfPageUp, n.HalfPageDown,
		n.Top, n.Bottom, n.Insert, n.Append,
	})
}

// Full help lists the keys of the mode the picker is in, with no two on one
// key, and the normal-mode keys only for a picker with modes.
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
	normal := []string{"G/end", "a", "^b/pgup", "^d", "^f/pgdn", "^u", "enter", "esc", "g/home", "i", "j/↓", "k/↑"}
	scoped := []string{"shift+tab", "tab"}
	tests := []struct {
		name string
		opts []Option
		keys []tea.Msg
		want []string
	}{
		{"without modes", nil, nil, typing},
		{"normal mode", []Option{WithModes(true)}, nil, normal},
		{"scopes in normal mode", []Option{WithModes(true), WithScopes("A")}, nil, append(slices.Clone(normal), scoped...)},
		{"insert mode", []Option{WithModes(true)}, []tea.Msg{tea.KeyPressMsg{Code: 'i', Text: "i"}}, typing},
		{"normal mode without a filter line", []Option{WithModes(true), WithFilterLine(false)}, nil, slices.DeleteFunc(slices.Clone(normal), func(k string) bool { return k == "i" || k == "a" })},
		{"scopes in insert mode", []Option{WithModes(true), WithScopes("A")}, []tea.Msg{tea.KeyPressMsg{Code: 'i', Text: "i"}}, append(slices.Clone(typing), scoped...)},
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

// Esc leaves insert mode, so its help says so there, and says close in
// normal mode and in a picker without modes.
func TestCancelHelp(t *testing.T) {
	descs := func(m Model) (short, full string) {
		for _, b := range m.ShortHelp() {
			if b.Help().Key == "esc" {
				short = b.Help().Desc
			}
		}
		for _, g := range m.FullHelp() {
			for _, b := range g {
				if b.Help().Key == "esc" {
					full = b.Help().Desc
				}
			}
		}
		return short, full
	}
	m := open(t, nil, WithItems(catalog), WithModes(true))
	if s, f := descs(m); s != "close" || f != "close" {
		t.Errorf("normal mode: short %q, full %q; want close", s, f)
	}
	m, _ = press(t, m, letter('i'))
	if s, f := descs(m); s != "normal" || f != "normal" {
		t.Errorf("insert mode: short %q, full %q; want normal", s, f)
	}
	plain := open(t, nil, WithItems(catalog))
	if s, f := descs(plain); s != "close" || f != "close" {
		t.Errorf("without modes: short %q, full %q; want close", s, f)
	}
}

// A picker without modes lists what its key map lists, and nothing of normal
// mode.
func TestHelpWithoutModesIsTheKeyMap(t *testing.T) {
	m := open(t, nil, WithItems(catalog))
	got, want := m.FullHelp(), m.KeyMap().FullHelp()
	if len(got) != len(want) {
		t.Fatalf("%d groups, want %d", len(got), len(want))
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			t.Errorf("group %d has %d bindings, want %d", i, len(got[i]), len(want[i]))
		}
	}
	if got, want := len(m.ShortHelp()), len(m.KeyMap().ShortHelp()); got != want {
		t.Errorf("short help has %d bindings, want %d", got, want)
	}
}
