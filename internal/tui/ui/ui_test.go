package ui

import (
	"context"
	"errors"
	"slices"
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

// The read that follows a kept page of the same query asks GitHub, and
// only that one: the others may be served the kept page.
func TestFeedPagesRereadsKeptPage(t *testing.T) {
	var (
		rereads []bool
		filter  = "a"
	)
	query := func(cursor string) string { return filter + "?" + cursor }
	fetch := FeedPages("list.test", new(Offline), query, func(_ context.Context, _ string, again bool) (core.Page[int], error) {
		rereads = append(rereads, again)
		return core.Page[int]{Items: []int{1}, Stale: !again}, nil
	})
	_, _, err := fetch(t.Context(), "")
	if !errors.Is(err, feed.ErrStale) {
		t.Fatalf("first read error = %v, want feed.ErrStale", err)
	}
	if _, _, err := fetch(t.Context(), "other"); !errors.Is(err, feed.ErrStale) {
		t.Fatalf("read of another page error = %v, want feed.ErrStale", err)
	}
	if _, _, err := fetch(t.Context(), ""); err != nil {
		t.Fatalf("read again error = %v, want none", err)
	}
	if _, _, err := fetch(t.Context(), ""); !errors.Is(err, feed.ErrStale) {
		t.Fatalf("later read error = %v, want feed.ErrStale", err)
	}
	// The first page of another filter isn't read with the flag that the
	// old filter's kept page left.
	filter = "b"
	if _, _, err := fetch(t.Context(), ""); !errors.Is(err, feed.ErrStale) {
		t.Fatalf("read of another filter error = %v, want feed.ErrStale", err)
	}
	if want := []bool{false, false, true, false, false}; !slices.Equal(rereads, want) {
		t.Errorf("reads marked to read again = %v, want %v", rereads, want)
	}
}

func TestFeedPages(t *testing.T) {
	var off Offline
	pages := map[string]core.Page[int]{
		"":      {Items: []int{1}, Next: "stale", Stale: true},
		"stale": {Items: []int{2}, Next: "off", Offline: true},
		"off":   {Items: []int{3}},
	}
	fetch := FeedPages("list.test", &off, func(cursor string) string { return cursor }, func(_ context.Context, cursor string, _ bool) (core.Page[int], error) {
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

	// A page served because GitHub rate limited the read marks it too.
	var limited Offline
	fetch = FeedPages("list.test", &limited, func(cursor string) string { return cursor }, func(context.Context, string, bool) (core.Page[int], error) {
		return core.Page[int]{Items: []int{4}, Limited: true}, nil
	})
	if items, _, err := fetch(t.Context(), ""); err != nil || len(items) != 1 {
		t.Errorf("limited page = %v, %v; want its items", items, err)
	}
	cmd = limited.Notify()
	if cmd == nil {
		t.Fatal("Notify after a limited page = nil, want a toast")
	}
	if msg, ok := cmd().(NotifyMsg); !ok || msg.Text != LimitedText {
		t.Errorf("toast = %v, want the rate-limited text", cmd())
	}
}
