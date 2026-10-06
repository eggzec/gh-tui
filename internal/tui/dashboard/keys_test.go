package dashboard

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
}

// The layers take a key in the order the dashboard does: the focused
// pane's keys and its own before those of the list or the calendar. Each
// row checks the binding the key reaches, and then what the key does.
func TestKeyLayersOrder(t *testing.T) {
	tests := []struct {
		to   []string
		key  string
		want string
		did  func(s *Section, before *Section, msgs []tea.Msg) bool
	}{
		{nil, "j", "Repositories: down", func(s, b *Section, _ []tea.Msg) bool { return selectedRepo(s) != selectedRepo(b) }},
		{nil, "enter", "Repositories: open", func(_, _ *Section, msgs []tea.Msg) bool { return hasRepoMsg(msgs) }},
		{nil, "]", "Repositories: next owner", func(s, b *Section, _ []tea.Msg) bool { return s.repos.cur != b.repos.cur }},
		{nil, "1", "Dashboard: focus pane", func(s, _ *Section, _ []tea.Msg) bool { return s.focus == pinnedPane }},
		// The list asks the app to open the filter; its keys don't page.
		{nil, "f", "Repositories: filter", func(s, b *Section, msgs []tea.Msg) bool {
			return selectedRepo(s) == selectedRepo(b) && opensFilter(msgs, filterform.FiltersTab)
		}},
		{nil, "s", "Repositories: sort", func(s, b *Section, msgs []tea.Msg) bool {
			return selectedRepo(s) == selectedRepo(b) && opensFilter(msgs, filterform.SortTab)
		}},
		{nil, "F", "nothing", func(s, b *Section, msgs []tea.Msg) bool { return selectedRepo(s) == selectedRepo(b) && len(msgs) == 0 }},
		{[]string{"tab"}, "j", "Work: down", func(s, b *Section, _ []tea.Msg) bool {
			return s.tasks.tabs[s.tasks.cur].sel != b.tasks.tabs[b.tasks.cur].sel
		}},
		{[]string{"tab"}, "]", "Work: next list", func(s, b *Section, _ []tea.Msg) bool { return s.tasks.cur != b.tasks.cur }},
		{[]string{"tab", "tab"}, "k", "Contributions: day before", func(s, b *Section, _ []tea.Msg) bool { return selectedDay(s) != selectedDay(b) }},
		{[]string{"tab", "tab"}, "tab", "Dashboard: next pane", func(s, _ *Section, _ []tea.Msg) bool { return s.focus == inboxPane }},
	}
	for _, tt := range tests {
		s := newSection(t, newFake(), nil, 80, 22)
		b := newSection(t, newFake(), nil, 80, 22)
		press(t, s, tt.to...)
		press(t, b, tt.to...)
		b2, src, ok := uitest.Winner(s.KeyLayers(), tt.key)
		got := "nothing"
		if ok {
			got = src + ": " + b2.Help().Desc
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

// opensFilter reports whether msgs ask the app to open the filter on tab.
func opensFilter(msgs []tea.Msg, tab filterform.Tab) bool {
	return len(msgs) == 1 && msgs[0] == ui.OpenFilterMsg{Tab: tab}
}

func selectedRepo(s *Section) string {
	r, _ := s.repos.selected()
	return r.Ref.String()
}

func selectedDay(s *Section) string {
	d, _ := s.cal.Selected()
	return fmt.Sprint(d)
}

func hasRepoMsg(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(ui.RepoMsg); ok {
			return true
		}
	}
	return false
}
