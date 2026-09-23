package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

func TestUpdateDetail(t *testing.T) {
	tests := []struct {
		name  string
		setup func(svc *fakeService)
		keys  []string
		check func(t *testing.T, s *Section, svc *fakeService, msgs []any)
	}{
		{
			name: "select opens the selected pull request with its comments",
			keys: []string{"down", "enter"},
			check: func(t *testing.T, s *Section, svc *fakeService, _ []any) {
				t.Helper()
				if s.thread == nil || s.detail.Number != 135 {
					t.Fatalf("open = %v with #%d, want #135 open", s.thread != nil, s.detail.Number)
				}
				if got := svc.got(); !slices.Equal(got, []int{135}) {
					t.Errorf("got details %v, want [135]", got)
				}
				view := screen(s)
				for _, want := range []string{"Retry GraphQL requests", "Cold starts read every page", "Does this survive a crash"} {
					if !strings.Contains(view, want) {
						t.Errorf("detail doesn't show %q:\n%s", want, view)
					}
				}
			},
		},
		{
			name: "back returns to the list with the selection kept",
			keys: []string{"down", "down", "enter", "esc"},
			check: func(t *testing.T, s *Section, _ *fakeService, _ []any) {
				t.Helper()
				if s.thread != nil {
					t.Fatal("detail still open")
				}
				if pr, _ := s.feed.Selected(); pr.Number != 128 || !s.feed.Focused() {
					t.Errorf("selected #%d, focused %v; want #128 focused", pr.Number, s.feed.Focused())
				}
			},
		},
		{
			name: "a cached detail shows at once",
			setup: func(svc *fakeService) {
				svc.cached[142] = true
				svc.getErr = errors.New("offline")
			},
			keys: []string{"enter"},
			check: func(t *testing.T, s *Section, _ *fakeService, msgs []any) {
				t.Helper()
				if !strings.Contains(screen(s), "Cold starts read every page") {
					t.Errorf("cached body not shown:\n%s", screen(s))
				}
				if len(msgs) == 0 {
					t.Error("a failed revalidation should notify")
				}
			},
		},
		{
			name:  "a failed detail keeps the list item and notifies",
			setup: func(svc *fakeService) { svc.getErr = errors.New("502 Bad Gateway") },
			keys:  []string{"enter"},
			check: func(t *testing.T, s *Section, _ *fakeService, msgs []any) {
				t.Helper()
				if !strings.Contains(screen(s), "Add a disk layer to the cache") {
					t.Errorf("header missing:\n%s", screen(s))
				}
				want := ui.NotifyMsg{Level: toast.Error, Text: "Couldn't load #142: 502 Bad Gateway"}
				if len(msgs) != 1 || msgs[0] != any(want) {
					t.Errorf("messages %v, want %v", msgs, want)
				}
			},
		},
		{
			name: "refresh fetches the detail and the comments again",
			keys: []string{"enter", "r"},
			check: func(t *testing.T, _ *Section, svc *fakeService, _ []any) {
				t.Helper()
				if got := svc.got(); len(got) != 2 {
					t.Errorf("got details %v, want two", got)
				}
				if n := len(svc.comments); n < 3 {
					t.Errorf("fetched %d comment pages, want the two again", n)
				}
			},
		},
		{
			name: "open in browser opens the open pull request",
			keys: []string{"down", "down", "enter", "o"},
			check: func(t *testing.T, _ *Section, _ *fakeService, msgs []any) {
				t.Helper()
				want := ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/pull/128"}
				if !slices.Contains(msgs, any(want)) {
					t.Errorf("messages %v, want %v", msgs, want)
				}
			},
		},
		{
			name: "filter doesn't apply in the detail",
			keys: []string{"enter", "f"},
			check: func(t *testing.T, s *Section, _ *fakeService, _ []any) {
				t.Helper()
				if s.filter != core.StateOpen || s.thread == nil {
					t.Errorf("filter = %s with detail open %v, want open with the detail", s.filter, s.thread != nil)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			if tt.setup != nil {
				tt.setup(svc)
			}
			s := started(t, svc, 80, 40)
			var msgs []any
			for _, k := range tt.keys {
				for _, m := range press(t, s, k) {
					switch m.(type) {
					case ui.OpenMsg, ui.NotifyMsg:
						msgs = append(msgs, m)
					}
				}
			}
			tt.check(t, s, svc, msgs)
		})
	}
}

func TestRepoMsgClosesDetail(t *testing.T) {
	s := started(t, newFakeService(), 80, 20)
	press(t, s, "enter")
	drain(t, s, s.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
	if s.thread != nil {
		t.Error("a new repository should close the detail")
	}
}

func TestHelpFollowsTheView(t *testing.T) {
	s := started(t, newFakeService(), 80, 20)
	descs := func() []string {
		short := s.Help().ShortHelp()
		d := make([]string, 0, len(short))
		for _, b := range short {
			d = append(d, b.Help().Desc)
		}
		return d
	}
	if got := descs(); !slices.Contains(got, "filter") || slices.Contains(got, "back") {
		t.Errorf("list help = %v, want filter and no back", got)
	}
	press(t, s, "enter")
	if got := descs(); slices.Contains(got, "filter") || !slices.Contains(got, "back") {
		t.Errorf("detail help = %v, want back and no filter", got)
	}
	// The bubbles lose the keys the section takes.
	for _, g := range s.Help().FullHelp() {
		for _, b := range g {
			if b.Help().Desc != "refresh" && slices.Contains(b.Keys(), "r") {
				t.Errorf("%q also claims r", b.Help().Desc)
			}
		}
	}
	if key.Matches(keyMsg("f"), s.keys.feed.PageDown) {
		t.Error("the feed's page down still takes the filter key")
	}
}

func TestCommentsWrapWithinWidth(t *testing.T) {
	s := started(t, newFakeService(), 40, 20)
	c := core.Comment{Author: core.User{Login: "octocat"}, Body: strings.Repeat("word ", 40), CreatedAt: clock}
	for l := range strings.SplitSeq(s.renderComment(c, 40), "\n") {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("line is %d wide: %q", w, ansi.Strip(l))
		}
	}
}
