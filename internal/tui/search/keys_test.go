package search

import (
	"fmt"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
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
	results, kinds := []string{"t", "e", "a", "down"}, []string{"t", "e", "a", "down", "left"}
	back := func(_, _ *Section, msgs []tea.Msg) bool { return slices.Contains(msgs, tea.Msg(ui.BackMsg{})) }
	tests := []struct {
		to   []string
		key  string
		want string
		did  func(s, before *Section, msgs []tea.Msg) bool
	}{
		{nil, "down", "query: results", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea }},
		{[]string{"t"}, "enter", "query: search", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea && s.text == "t" }},
		{nil, "j", "nothing", func(s, _ *Section, _ []tea.Msg) bool { return s.input.Value() == "j" }},
		{nil, "esc", "query: back", back},
		// The query types ] and [ before the moves that hold them, whose
		// other keys still move.
		{nil, "]", "nothing", func(s, _ *Section, _ []tea.Msg) bool { return s.input.Value() == "]" && s.area == inputArea }},
		{nil, "[", "nothing", func(s, _ *Section, _ []tea.Msg) bool { return s.input.Value() == "[" && s.area == inputArea }},
		{nil, "tab", "query: next", func(s, _ *Section, _ []tea.Msg) bool { return s.area == kindsArea }},
		{nil, "shift+tab", "query: previous", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea }},
		{results, "j", "results: down", func(s, b *Section, _ []tea.Msg) bool { return selectedHit(s) != selectedHit(b) }},
		{results, "enter", "search: open", func(_, _ *Section, msgs []tea.Msg) bool { return len(msgs) > 0 }},
		{results, "left", "search: kinds", func(s, _ *Section, _ []tea.Msg) bool { return s.area == kindsArea }},
		{results, "esc", "search: back", back},
		// The app opens the filter; its keys don't page the results.
		{results, "f", "search: filter", func(s, b *Section, msgs []tea.Msg) bool { return selectedHit(s) == selectedHit(b) && len(msgs) == 0 }},
		{results, "s", "search: sort", func(s, b *Section, msgs []tea.Msg) bool { return selectedHit(s) == selectedHit(b) && len(msgs) == 0 }},
		{kinds, "j", "search: down", func(s, b *Section, _ []tea.Msg) bool { return s.kind != b.kind }},
		{kinds, "enter", "search: results", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea }},
		{kinds, "tab", "search: next", func(s, _ *Section, _ []tea.Msg) bool { return s.area == resultsArea }},
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
