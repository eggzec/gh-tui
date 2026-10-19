package orderpick

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, testKeys(t))
	keytest.Tagged(t, testKeys(t))
	keytest.HelpTags(t, testKeys(t))
}

// The key map shares keys between the panes and the modes, so only the
// picker in each state is free of conflicts.
func TestHelpByState(t *testing.T) {
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
	moves := []string{"G/end", "^b/pgup", "^d", "^f/pgdn", "g/home", "j/↓", "k/↑", "^u"}
	keys := func(extra ...string) []string {
		all := slices.Concat(moves, extra)
		slices.Sort(all)
		return all
	}
	tests := []struct {
		name string
		keys []any
		want []string
	}{
		{"left", nil, keys("/", "space", "↵", "esc", "tab", "shift+tab")},
		{"search", []any{letter('/')}, []string{"↑/^p", "↓/^n", "↵", "esc", "pgdn", "pgup"}},
		{"right", []any{tab}, keys("J", "K", "space", "x/delete", "↵", "esc", "tab", "shift+tab")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t)
			for _, k := range tt.keys {
				m, _ = press(t, m, k)
			}
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if got := enabled(m); !slices.Equal(got, want) {
				t.Errorf("enabled %q, want %q", got, want)
			}
			keytest.NoConflicts(t, m)
			short := m.ShortHelp()
			if len(short) == 0 || slices.ContainsFunc(short, func(b key.Binding) bool { return !b.Enabled() }) {
				t.Errorf("short help %v has no bindings or a disabled one", short)
			}
		})
	}
}

// The unused keys of the picker stay disabled, so no key is taken twice.
func TestUnusedKeysAreDisabled(t *testing.T) {
	k := testKeys(t)
	for _, b := range []key.Binding{k.Picker.NextScope, k.Picker.PrevScope, k.Picker.Normal.Back} {
		if b.Enabled() {
			t.Errorf("%v is enabled", b.Help())
		}
	}
}
