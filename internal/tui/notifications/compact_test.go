package notifications

import (
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func newWith(tb testing.TB, svc Service, width, height int, opts ...Option) *Section {
	tb.Helper()
	opts = append([]Option{WithNow(func() time.Time { return now })}, opts...)
	s := New(tb.Context(), svc, config.Default().Keys, opts...)
	p, err := config.Default().Palette(true)
	if err != nil {
		tb.Fatal(err)
	}
	s.SetTheme(ui.NewTheme(p, true))
	s.SetSize(width, height)
	s.Focus()
	tb.Cleanup(func() { closeQuestion(s, nil) })
	run(tb, s, s.Init())
	return s
}

func TestWithoutHeaderUsesTheWholeHeight(t *testing.T) {
	t.Parallel()
	full := newWith(t, newFake(inbox()...), 40, 6)
	compact := newWith(t, newFake(inbox()...), 40, 6, WithoutHeader())

	if got := len(strings.Split(compact.View(), "\n")); got != 6 {
		t.Errorf("compact view has %d lines, want 6", got)
	}
	if strings.Contains(compact.View(), "filter") {
		t.Errorf("compact view has the filter line:\n%s", compact.View())
	}
	if full.View() == compact.View() {
		t.Error("compact view equals the full one")
	}
	if got, want := compact.feed.View(), compact.View(); got != want {
		t.Errorf("compact view is not the bare list:\n%s", got)
	}
	if got := len(strings.Split(full.View(), "\n")); got != 6 {
		t.Errorf("full view has %d lines, want 6", got)
	}
}

func TestWithPageLimitKeepsTheCursor(t *testing.T) {
	t.Parallel()
	svc := newFake(manyThreads(7)...)
	svc.size = 2
	s := newWith(t, svc, 80, 12, WithPageLimit(1))
	before := svc.listCount()
	p, err := s.list(t.Context(), s.query(""), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := svc.listCount() - before; got != 1 {
		t.Errorf("%d reads, want 1", got)
	}
	if len(p.Items) != 2 || p.Next == "" {
		t.Errorf("got %d items, next %q; want 2 items and a cursor", len(p.Items), p.Next)
	}
}

func TestPageLimitZeroIsTheDefault(t *testing.T) {
	t.Parallel()
	svc := newFake(manyThreads(7)...)
	svc.size = 2
	s := newWith(t, svc, 80, 12, WithPageLimit(0))
	if got := len(loaded(s)); got != 7 {
		t.Fatalf("loaded %d threads, want 7", got)
	}
}
