package filterform

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// The key map shares enter and esc between the rows, insert mode and the
// dropdowns, so only the form in each state is free of conflicts.
func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, DefaultKeyMap())
}

// Full help enables the keys that act in the mode and on the row in focus,
// with no two on one key.
func TestFullHelpState(t *testing.T) {
	// The keys every row takes, by their help keys: the moves, apply, close
	// and the tabs.
	always := []string{"k", "j", "g", "G", "↵", "esc", "q", "[", "]"}
	with := func(extra ...string) []string { return slices.Concat(always, extra) }
	// What a dropdown in its normal mode takes besides its own list keys:
	// the moves, enter and esc, q, and the tabs.
	listNormal := []string{"k/↑", "j/↓", "^b/pgup", "^f/pgdn", "^u", "^d", "g/home", "G/end", "↵", "esc", "q", "[", "]"}
	list := func(extra ...string) []string { return slices.Concat(listNormal, extra) }
	tests := []struct {
		name string
		opts []Option
		keys []tea.Msg
		want []string
		// load gives the labels a loader, which fails when fail is set, or
		// is waited on when block is.
		load, fail, block bool
	}{
		{name: "choice row", want: with("h", "l", "space", "delete")},
		{name: "toggle row", keys: keys(down, rowDrafts), want: with("h", "l", "space", "delete")},
		{name: "person row", keys: keys(down, rowAuthor), want: with("space", "delete")},
		{name: "multi row", keys: keys(down, rowLabels), want: with("space", "delete")},
		{name: "text row", keys: keys(down, rowBase), want: with("i", "a", "delete")},
		{name: "query line", keys: keys(down, rowQuery), want: with("i", "a")},
		{name: "sort by", opts: []Option{WithTab(SortTab)}, want: with("h", "l", "space")},
		{name: "order", opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{down}, want: with("h", "l", "space")},
		{name: "sort query line", opts: []Option{WithTab(SortTab)}, keys: keys(down, sortRows), want: with("i", "a")},
		{name: "text insert", keys: append(keys(down, rowBase), keyA), want: []string{"↵", "esc"}},
		{name: "query insert", keys: []tea.Msg{keyBigG, keyI}, want: []string{"↵", "esc"}},
		{name: "list", keys: []tea.Msg{space}, want: list("delete")},
		{name: "list of the sort by", opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{space}, want: list()},
		{name: "list of the order", opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{down, space}, want: list()},
		{name: "people", keys: keys2(down, rowAuthor, space), want: list("i", "a", "delete")},
		{name: "people typing", keys: keys2(down, rowAuthor, space, keyI), want: []string{"↑", "↓", "pgup", "pgdn", "↵", "esc"}},
		{name: "checklist", load: true, keys: keys2(down, rowLabels, space), want: list("space", "i", "a", "delete")},
		{name: "checklist typing", load: true, keys: keys2(down, rowLabels, space, keyI), want: []string{"↑", "↓", "pgup", "pgdn", "↵", "esc"}},
		{name: "checklist without a filter", keys: keys2(down, rowLabels, space), want: list("space", "delete")},
		{name: "loading", load: true, block: true, keys: keys2(down, rowLabels, space), want: []string{"esc", "q", "[", "]"}},
		{name: "failed load", load: true, fail: true, keys: keys2(down, rowLabels, space), want: []string{"r", "esc", "q", "[", "]"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLoader{}
			var load Loader
			if tt.load {
				load = f.load
			}
			if tt.fail {
				f.fail = errBoom
			}
			m := open(t, prSpec(load), tt.opts...)
			if tt.block {
				f.block = make(chan struct{})
				t.Cleanup(func() { close(f.block) })
				// The load is on its way, and its result isn't fed back.
				m, _ = press(t, m, keys(down, rowLabels)...)
				m, _ = m.Update(space)
			} else {
				m, _ = press(t, m, tt.keys...)
			}
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

// keys2 is n presses of k, then more.
func keys2(k tea.Msg, n int, more ...tea.Msg) []tea.Msg {
	return append(keys(k, n), more...)
}
