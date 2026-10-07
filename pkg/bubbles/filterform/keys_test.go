package filterform

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// The key map shares enter and esc between the rows, insert mode and the
// picker, so only the form in each state is free of conflicts.
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

// Full help enables the keys that act in the mode and on the row in focus,
// with no two on one key.
func TestFullHelpState(t *testing.T) {
	// The keys every row takes, by their help keys: the moves, apply, close
	// and the tabs.
	always := []string{"k", "j", "g", "G", "↵", "esc", "q", "[", "]"}
	with := func(extra ...string) []string { return slices.Concat(always, extra) }
	tests := []struct {
		name string
		opts []Option
		keys []tea.Msg
		want []string
	}{
		{name: "choice row", want: with("h", "l", "delete")},
		{name: "toggle row", keys: keys(down, rowDrafts), want: with("h", "l", "space", "delete")},
		{name: "person row", keys: keys(down, rowAuthor), want: with("space", "delete")},
		{name: "multi row", keys: keys(down, rowLabels), want: with("space", "delete")},
		{name: "text row", keys: keys(down, rowBase), want: with("i", "a", "delete")},
		{name: "query line", keys: keys(down, rowQuery), want: with("i", "a")},
		{name: "sort by", opts: []Option{WithTab(SortTab)}, want: with("h", "l")},
		{name: "order", opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{down}, want: with("h", "l")},
		{name: "sort query line", opts: []Option{WithTab(SortTab)}, keys: keys(down, sortRows), want: with("i", "a")},
		{name: "text insert", keys: append(keys(down, rowBase), keyA), want: []string{"↵", "esc"}},
		{name: "query insert", keys: []tea.Msg{keyBigG, keyI}, want: []string{"↵", "esc"}},
		{name: "picker", keys: append(keys(down, rowAuthor), space), want: []string{"space", "↵", "esc", "↑/ctrl+p", "↓/ctrl+n", "pgup", "pgdn"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := press(t, open(t, prSpec(nil), tt.opts...), tt.keys...)
			var got []string
			for _, g := range m.FullHelp() {
				for _, b := range g {
					if b.Enabled() {
						got = append(got, b.Help().Key)
					}
				}
			}
			// Apply and Typing.Submit share a label, as do Cancel and Leave
			// by their keys, so count each once.
			slices.Sort(got)
			got = slices.Compact(got)
			want := slices.Sorted(slices.Values(tt.want))
			want = slices.Compact(want)
			if !slices.Equal(got, want) {
				t.Errorf("enabled %q, want %q", got, want)
			}
			keytest.NoConflicts(t, m)
		})
	}
}

// A form without a sort lists no tab keys, and a choice with no options
// no keys to change it.
func TestFullHelpWithoutTabsOrOptions(t *testing.T) {
	enabled := func(m Model) []string {
		var out []string
		for _, g := range m.FullHelp() {
			for _, b := range g {
				if b.Enabled() {
					out = append(out, b.Help().Key)
				}
			}
		}
		return out
	}
	m := open(t, noSortSpec(nil))
	if got := enabled(m); slices.Contains(got, "[") || slices.Contains(got, "]") {
		t.Errorf("enabled %q, want no tab keys", got)
	}
	empty := open(t, Spec{Fields: []Field{{Key: "wf", Label: "Workflow", Kind: Choice, Qualifier: "workflow"}}})
	if got := enabled(empty); slices.Contains(got, "h") || slices.Contains(got, "l") {
		t.Errorf("enabled %q, want no change keys", got)
	}
}

// A failed load enables the retry key, and nothing else does.
func TestFullHelpRetry(t *testing.T) {
	f := &fakeLoader{fail: errBoom}
	m := open(t, prSpec(f.load))
	retry := func(m Model) bool {
		for _, g := range m.FullHelp() {
			for _, b := range g {
				if b.Help().Desc == "retry" && b.Enabled() {
					return true
				}
			}
		}
		return false
	}
	if retry(m) {
		t.Error("retry is enabled in the rows")
	}
	m, _ = press(t, m, down, down, down, space)
	if !retry(m) {
		t.Error("retry is disabled after a load failed")
	}
	keytest.NoConflicts(t, m)
}
