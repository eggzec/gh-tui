package ui

import (
	"context"
	"errors"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

func TestBinding(t *testing.T) {
	keys := map[string][]string{"merge": {"m", "ctrl+m"}, "open": {"enter"}}

	b := Binding(keys, "merge", "merge")
	if !b.Enabled() || b.Help().Key != "m" || b.Help().Desc != "merge" {
		t.Errorf("merge = %+v, want enabled with help m/merge", b.Help())
	}
	if !key.Matches(tea.KeyPressMsg{Code: 'm', Mod: tea.ModCtrl}, b) {
		t.Error("ctrl+m doesn't match merge")
	}
	if got := Binding(keys, "open", "open").Help().Key; got != "↵" {
		t.Errorf("enter label = %q, want ↵", got)
	}
	if Binding(keys, "missing", "x").Enabled() {
		t.Error("an action without keys should be disabled")
	}
}

type opFunc func(context.Context) error

func (f opFunc) Do(ctx context.Context) error { return f(ctx) }

func TestDo(t *testing.T) {
	errNo := errors.New("no")
	msg := Do(t.Context(), IssuesTitle, opFunc(func(context.Context) error { return errNo }), "close #7")()
	done, ok := msg.(DoneMsg)
	if !ok || done.From != IssuesTitle || done.What != "close #7" || !errors.Is(done.Err, errNo) {
		t.Errorf("Do = %#v, want DoneMsg for close #7 with the error", msg)
	}
}

func TestThemeTakesPaletteColors(t *testing.T) {
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	th := NewTheme(p, true)
	accent := lipgloss.Color(p.Accent)
	checks := map[string]any{
		"feed cursor":  th.Feed().Cursor.GetForeground(),
		"thread key":   th.Thread().Key.GetForeground(),
		"prompt edge":  th.Prompt().Frame.GetBorderLeftForeground(),
		"prompt caret": th.Prompt().Cursor.GetForeground(),
		"toast info":   th.Toast().Info.Color,
		"tree cursor":  th.Tree().Cursor.GetForeground(),
		"pager prompt": th.Pager().Prompt.GetForeground(),
		"accent text":  th.Accent.GetForeground(),
		"picker match": th.Picker().Match.GetForeground(),
	}
	for name, got := range checks {
		if got != accent {
			t.Errorf("%s = %v, want the palette accent %v", name, got, accent)
		}
	}
	if got := th.Prompt().BlurredFrame.GetBorderLeftForeground(); got != lipgloss.Color(p.Border) {
		t.Errorf("blurred prompt edge = %v, want the palette border color", got)
	}
	if got := th.Prompt().Text.GetForeground(); got != lipgloss.Color(p.Foreground) {
		t.Errorf("prompt text = %v, want the palette foreground", got)
	}
	if th.Pager().Syntax == nil {
		t.Error("pager has no syntax colors")
	}
	if got := th.Toast().Error.Color; got != lipgloss.Color(p.Error) {
		t.Errorf("toast error = %v, want the palette error color", got)
	}
}

func TestFeedPages(t *testing.T) {
	var off Offline
	pages := map[string]core.Page[int]{
		"":      {Items: []int{1}, Next: "stale", Stale: true},
		"stale": {Items: []int{2}, Next: "off", Offline: true},
		"off":   {Items: []int{3}},
	}
	fetch := FeedPages(&off, func(_ context.Context, cursor string) (core.Page[int], error) {
		if cursor == "fail" {
			return core.Page[int]{}, errors.New("boom")
		}
		return pages[cursor], nil
	})
	if items, next, err := fetch(t.Context(), ""); !errors.Is(err, feed.ErrStale) || len(items) != 1 || next != "stale" {
		t.Errorf("stale page = %v, %q, %v; want its items with feed.ErrStale", items, next, err)
	}
	if off.Notify() != nil {
		t.Error("Notify before any offline page returned a toast")
	}
	if items, _, err := fetch(t.Context(), "stale"); err != nil || len(items) != 1 {
		t.Errorf("offline page = %v, %v; want its items", items, err)
	}
	cmd := off.Notify()
	if cmd == nil {
		t.Fatal("Notify after an offline page = nil, want a toast")
	}
	if msg, ok := cmd().(NotifyMsg); !ok || msg.Text != OfflineText {
		t.Errorf("toast = %v, want the offline text", cmd())
	}
	if off.Notify() != nil {
		t.Error("second Notify returned a toast, want one only")
	}
	if _, _, err := fetch(t.Context(), "fail"); err == nil || errors.Is(err, feed.ErrStale) {
		t.Errorf("failed read error = %v, want it passed on", err)
	}
}
