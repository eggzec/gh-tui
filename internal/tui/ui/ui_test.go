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

func TestOpenHint(t *testing.T) {
	keys := map[string][]string{"open": {"o"}}
	if got := OpenHint(Binding(keys, "open", "open")); got != "o to open on GitHub" {
		t.Errorf("OpenHint = %q", got)
	}
	if got := OpenHint(Binding(keys, "missing", "open")); got != "" {
		t.Errorf("OpenHint without a key = %q, want nothing", got)
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
	ic := NewIcons(config.IconsNerd)
	accent := lipgloss.Color(p.Accent)
	checks := map[string]any{
		"feed cursor":  th.Feed(ic).Cursor.GetForeground(),
		"thread key":   th.Thread(ic).Key.GetForeground(),
		"prompt edge":  th.Prompt().Frame.GetBorderLeftForeground(),
		"prompt caret": th.Prompt().Cursor.GetForeground(),
		"toast info":   th.Toast(ic).Info.Color,
		"tree cursor":  th.Tree(ic).Cursor.GetForeground(),
		"pager prompt": th.Pager(ic).Prompt.GetForeground(),
		"accent text":  th.Accent.GetForeground(),
		"picker match": th.Picker(ic).Match.GetForeground(),
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
	if th.Pager(ic).Syntax == nil {
		t.Error("pager has no syntax colors")
	}
	if got := th.Toast(ic).Error.Color; got != lipgloss.Color(p.Error) {
		t.Errorf("toast error = %v, want the palette error color", got)
	}
}

// Every bubble that marks what went wrong takes the mark of the icons, as
// the error lines of the sections do.
func TestThemeTakesErrorGlyph(t *testing.T) {
	p, err := config.Default().Palette(false)
	if err != nil {
		t.Fatal(err)
	}
	th := NewTheme(p, false)
	// The Octicons x-circle-fill, as GitHub marks a failure; the ballot x;
	// and an x, since "!" marks the warnings of a log.
	want := map[string]string{config.IconsNerd: "\uf530", config.IconsUnicode: "✗", config.IconsASCII: "x"}
	for set, mark := range want {
		ic := NewIcons(set)
		if ic.Error != mark {
			t.Errorf("%s: error glyph = %q, want %q", set, ic.Error, mark)
		}
		got := map[string]string{
			"feed":        th.Feed(ic).ErrorGlyph,
			"thread":      th.Thread(ic).ErrorGlyph,
			"tree":        th.Tree(ic).ErrorGlyph,
			"graph":       th.Graph(ic).ErrorGlyph,
			"pager":       th.Pager(ic).ErrorGlyph,
			"logview":     th.LogView(ic).ErrorGlyph,
			"filterform":  th.FilterForm(ic).ErrorGlyph,
			"form picker": th.FilterForm(ic).Picker.ErrorGlyph,
			"picker":      th.Picker(ic).ErrorGlyph,
			"finder":      th.Finder(ic).ErrorGlyph,
			"error lines": th.Errors(ic).Mark,
			"toast":       th.Toast(ic).Error.Glyph,
		}
		for name, g := range got {
			if g != ic.Error {
				t.Errorf("%s: %s mark = %q, want %q", set, name, g, ic.Error)
			}
		}
		// The separator and the ellipsis take the same path.
		joins := map[string][2]string{
			"feed":        {th.Feed(ic).ErrorSeparator, th.Feed(ic).ErrorEllipsis},
			"thread":      {th.Thread(ic).ErrorSeparator, th.Thread(ic).ErrorEllipsis},
			"tree":        {th.Tree(ic).ErrorSeparator, th.Tree(ic).ErrorEllipsis},
			"graph":       {th.Graph(ic).ErrorSeparator, th.Graph(ic).ErrorEllipsis},
			"pager":       {th.Pager(ic).ErrorSeparator, th.Pager(ic).ErrorEllipsis},
			"logview":     {th.LogView(ic).ErrorSeparator, th.LogView(ic).ErrorEllipsis},
			"filterform":  {th.FilterForm(ic).ErrorSeparator, th.FilterForm(ic).ErrorEllipsis},
			"picker":      {th.Picker(ic).ErrorSeparator, th.Picker(ic).ErrorEllipsis},
			"finder":      {th.Finder(ic).ErrorSeparator, th.Finder(ic).ErrorEllipsis},
			"error lines": {th.Errors(ic).Separator, th.Errors(ic).Ellipsis},
			"empty lines": {th.Empty(ic).Separator, th.Empty(ic).Ellipsis},
		}
		for name, j := range joins {
			if j != [2]string{ic.Separator, ic.Ellipsis} {
				t.Errorf("%s: %s separator and ellipsis = %q, want %q and %q", set, name, j, ic.Separator, ic.Ellipsis)
			}
		}
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
	fetch := FeedPages("list.test", query, func(_ context.Context, _ string, again bool) (core.Page[int], error) {
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
	pages := map[string]core.Page[int]{
		"":      {Items: []int{1}, Next: "stale", Stale: true},
		"stale": {Items: []int{2}, Next: "off", Offline: true, Stale: true},
		"off":   {Items: []int{3}, Next: "limited"},
		// Served because GitHub rate limited the read.
		"limited": {Items: []int{4}, Limited: true, Stale: true},
	}
	fetch := FeedPages("list.test", func(cursor string) string { return cursor }, func(_ context.Context, cursor string, _ bool) (core.Page[int], error) {
		if cursor == "fail" {
			return core.Page[int]{}, errors.New("boom")
		}
		return pages[cursor], nil
	})
	if items, next, err := fetch(t.Context(), ""); !errors.Is(err, feed.ErrStale) || len(items) != 1 || next != "stale" {
		t.Errorf("stale page = %v, %q, %v; want its items with feed.ErrStale", items, next, err)
	}
	// A kept page served for want of GitHub isn't read again at once:
	// that read would be served the same. The feed reads it again once
	// GitHub answers, or the limit lifts.
	for _, cursor := range []string{"stale", "limited"} {
		if items, _, err := fetch(t.Context(), cursor); !errors.Is(err, feed.ErrKept) || len(items) != 1 {
			t.Errorf("page %q = %v, %v; want its items with feed.ErrKept", cursor, items, err)
		}
	}
	if _, _, err := fetch(t.Context(), "fail"); err == nil || errors.Is(err, feed.ErrStale) {
		t.Errorf("failed read error = %v, want it passed on", err)
	}
}

// TestYield checks that a binding keeps its keys beside an enabled one,
// and leaves a disabled one the keys they share.
func TestYield(t *testing.T) {
	refresh := key.NewBinding(key.WithKeys("r", "ctrl+r"), key.WithHelp("r", "refresh"))
	rerun := key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("^r", "rerun failed"))
	if got := Yield(refresh, rerun); !slices.Equal(got.Keys(), []string{"r", "ctrl+r"}) {
		t.Errorf("beside an enabled re-run, refresh holds %q", got.Keys())
	}
	rerun.SetEnabled(false)
	got := Yield(refresh, rerun)
	if !slices.Equal(got.Keys(), []string{"r"}) || got.Help() != refresh.Help() || !got.Enabled() {
		t.Errorf("beside a disabled re-run, refresh is %q %+v enabled %v, want r alone", got.Keys(), got.Help(), got.Enabled())
	}
	if Yield(rerun, rerun).Enabled() {
		t.Error("a binding left no key is enabled")
	}
}
