package dashboard

import (
	"fmt"
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
		{nil, "j", "list: down", func(s, b *Section, _ []tea.Msg) bool { return selectedRepo(s) != selectedRepo(b) }},
		{nil, "enter", "dashboard: open", func(_, _ *Section, msgs []tea.Msg) bool { return hasRepoMsg(msgs) }},
		{nil, "]", "dashboard: next owner", func(s, b *Section, _ []tea.Msg) bool { return s.repos.cur != b.repos.cur }},
		{nil, "1", "dashboard: focus pane", func(s, _ *Section, _ []tea.Msg) bool { return s.focus == pinnedPane }},
		{[]string{"tab"}, "j", "dashboard: down", func(s, b *Section, _ []tea.Msg) bool {
			return s.tasks.tabs[s.tasks.cur].sel != b.tasks.tabs[b.tasks.cur].sel
		}},
		{[]string{"tab"}, "]", "dashboard: next list", func(s, b *Section, _ []tea.Msg) bool { return s.tasks.cur != b.tasks.cur }},
		{[]string{"tab", "tab"}, "k", "calendar: day before", func(s, b *Section, _ []tea.Msg) bool { return selectedDay(s) != selectedDay(b) }},
		{[]string{"tab", "tab"}, "tab", "dashboard: next pane", func(s, _ *Section, _ []tea.Msg) bool { return s.focus == inboxPane }},
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
