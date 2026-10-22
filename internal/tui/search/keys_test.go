package search

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
}

// The layers take a key in the order the page does: the query types what
// its keys don't take, and the page's own keys come before the list's.
// Each row checks the binding the key reaches, and then what the key does.
func TestKeyLayersOrder(t *testing.T) {
	typing, results := []string{"i"}, []string{"i", "t", "e", "a", "enter"}
	tests := []struct {
		to   []string
		key  string
		want string
		did  func(s, before *Section, msgs []tea.Msg) bool
	}{
		// The page opens on the query, which takes no text yet.
		{nil, "j", "nothing", func(s, _ *Section, _ []tea.Msg) bool { return s.input.Value() == "" && !s.typing }},
		{nil, "i", "Search: type", func(s, _ *Section, _ []tea.Msg) bool { return s.typing && s.area == inputArea }},
		{nil, "]", "Search: next kind", func(s, _ *Section, _ []tea.Msg) bool { return s.kind == core.SearchIssues }},
		{nil, "[", "Search: previous kind", func(s, _ *Section, _ []tea.Msg) bool { return s.kind == core.SearchCommits }},
		{nil, "tab", "Search: next", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea && !s.typing }},
		{nil, "2", "Search: show kind", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea && s.kind == core.SearchIssues }},
		{nil, "enter", "Search: search", func(s, _ *Section, _ []tea.Msg) bool { return s.area == inputArea }},
		// While it types, the query takes printable keys, and tab stays in
		// it: esc is the way out.
		{typing, "j", "nothing", func(s, _ *Section, _ []tea.Msg) bool { return s.input.Value() == "j" && s.typing }},
		{typing, "]", "nothing", func(s, _ *Section, _ []tea.Msg) bool { return s.input.Value() == "]" && s.typing }},
		{typing, "1", "nothing", func(s, _ *Section, _ []tea.Msg) bool { return s.input.Value() == "1" && s.typing }},
		{typing, "tab", "nothing", func(s, _ *Section, _ []tea.Msg) bool { return s.typing && s.area == inputArea }},
		{typing, "esc", "Query: results", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea && !s.typing }},
		{[]string{"i", "t"}, "enter", "Query: search", func(s, _ *Section, _ []tea.Msg) bool {
			return s.area == resultsArea && s.text == "t" && !s.typing
		}},
		// In the results, the page's keys come first and the list's after.
		{results, "i", "Search: type", func(s, _ *Section, _ []tea.Msg) bool { return s.typing && s.input.Value() == "tea" }},
		{results, "1", "Search: show kind", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea && s.kind == core.SearchRepos }},
		{results, "tab", "Search: next", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea && s.preview && !s.typing }},
		{results, "]", "Search: next kind", func(s, _ *Section, _ []tea.Msg) bool { return s.kind == core.SearchIssues && s.area == resultsArea }},
		{results, "j", "Results: down", func(s, b *Section, _ []tea.Msg) bool {
			return selectedHit(s) != selectedHit(b) || s.starts.sel != b.starts.sel
		}},
		{results, "enter", "Results: open", func(_, _ *Section, msgs []tea.Msg) bool { return len(msgs) > 0 }},
		// The results ask the app to open the filter.
		{results, "f", "Results: filter", func(s, b *Section, msgs []tea.Msg) bool {
			return selectedHit(s) == selectedHit(b) && len(msgs) == 1 && msgs[0] == ui.OpenFilterMsg{}
		}},
		{results, "s", "Results: sort", func(s, b *Section, msgs []tea.Msg) bool {
			return selectedHit(s) == selectedHit(b) && len(msgs) == 1 && msgs[0] == ui.OpenSortMsg{}
		}},
	}
	for _, tt := range tests {
		s, b := newSection(t, newFake(), 120, 30), newSection(t, newFake(), 120, 30)
		s.Focus()
		b.Focus()
		press(t, s, tt.to...)
		press(t, b, tt.to...)
		got := "nothing"
		if w, src, ok := uitest.Winner(s.KeyLayers(), tt.key); ok {
			got = src + ": " + w.Help().Desc
		}
		if got != tt.want {
			t.Errorf("after %q, %s reaches %q, want %q", tt.to, tt.key, got, tt.want)
			continue
		}
		if msgs := press(t, s, tt.key); !tt.did(s, b, msgs) {
			t.Errorf("after %q, %s didn't do what %q says", tt.to, tt.key, tt.want)
		}
	}
}

// selectedHit names the result under the cursor, if any.
func selectedHit(s *Section) string {
	l, ok := s.visibleHits()
	if !ok {
		return ""
	}
	h, _ := l.feed.Selected()
	return fmt.Sprint(h)
}

// J and K step to the next and the previous hit, as j and k do.
func TestNextPrevHit(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	typeText(t, s, "tea")
	first := selectedHit(s)
	if first == "" {
		t.Fatal("the search found no hit")
	}
	press(t, s, "J")
	second := selectedHit(s)
	if second == first {
		t.Fatalf("J left the cursor on %s, want the next hit", first)
	}
	press(t, s, "j", "K")
	if got := selectedHit(s); got != second {
		t.Errorf("j then K left the cursor on %s, want %s", got, second)
	}
	press(t, s, "K")
	if got := selectedHit(s); got != first {
		t.Errorf("K moved to %s, want the first hit %s", got, first)
	}
}

// J types in the query, where it is no step to a hit.
func TestNextHitNeedsResultsFocus(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	typeText(t, s, "tea")
	press(t, s, "i")
	before := selectedHit(s)
	press(t, s, "J")
	if got := s.input.Value(); got != "teaJ" {
		t.Errorf("query = %q after J while typing, want teaJ", got)
	}
	if got := selectedHit(s); got != before {
		t.Errorf("J while typing moved the cursor to %s, want it on %s", got, before)
	}
}

// Before a query, the results are the suggestions, which move with the
// keys of the results' context, and the help lists those moves once.
func TestSuggestionsMoveWithTheResultsKeys(t *testing.T) {
	keys := config.Default().Keys
	keys.Set("search_results.down", []string{"w"})
	s := New(t.Context(), newFake(), keys, WithNow(func() time.Time { return now }), WithStart(startRepos))
	s.SetSize(120, 30)
	s.Focus()
	run(t, s, s.Init())
	press(t, s, "2")
	if s.area != resultsArea || len(s.starts.rows) < 2 {
		t.Fatalf("area %v with %d suggestions, want the results with several", s.area, len(s.starts.rows))
	}

	press(t, s, "j")
	if s.starts.sel != 0 {
		t.Error("j moved down, but down is bound to w in the results")
	}
	press(t, s, "w")
	if s.starts.sel != 1 {
		t.Errorf("w moved to suggestion %d, want 1", s.starts.sel)
	}

	downs := 0
	for _, l := range s.KeyLayers() {
		for _, b := range l.Bindings {
			if b.Help().Desc == "down" {
				downs++
			}
		}
	}
	if downs != 1 {
		t.Errorf("help lists down %d times in the results, want once", downs)
	}
}
