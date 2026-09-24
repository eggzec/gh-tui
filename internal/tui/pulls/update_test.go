package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The real service is what the app passes in.
var _ Service = (*pulls.Service)(nil)

// screen is the section's view with styles removed.
func screen(h *host) string {
	return ansi.Strip(h.View())
}

func TestUpdateList(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		// check inspects the section after keys, the service and the
		// messages the keys sent to the app.
		check func(t *testing.T, s *host, svc *fakeService, msgs []any)
	}{
		{
			name: "filter cycles open, closed, merged and back",
			keys: []string{"f", "f", "f"},
			check: func(t *testing.T, s *host, svc *fakeService, _ []any) {
				t.Helper()
				var states []core.State
				for _, q := range svc.listed() {
					if q.Cursor == "" {
						states = append(states, q.State)
					}
				}
				want := []core.State{core.StateOpen, core.StateClosed, core.StateMerged, core.StateOpen}
				if !slices.Equal(states, want) {
					t.Errorf("listed %v, want %v", states, want)
				}
				if s.filter != core.StateOpen {
					t.Errorf("filter = %s, want open", s.filter)
				}
			},
		},
		{
			name: "filter shows the current state in the header",
			keys: []string{"f"},
			check: func(t *testing.T, s *host, _ *fakeService, _ []any) {
				t.Helper()
				if !strings.Contains(screen(s), "#86") || strings.Contains(screen(s), "#142") {
					t.Errorf("closed filter shows\n%s", screen(s))
				}
				if !strings.Contains(s.header, s.st.filterOn.Render("closed")) {
					t.Errorf("header %q doesn't highlight closed", s.header)
				}
			},
		},
		{
			name: "refresh invalidates the repository, then lists the loaded pages again",
			keys: []string{"r"},
			check: func(t *testing.T, _ *host, svc *fakeService, _ []any) {
				t.Helper()
				if got, want := svc.invalidations(), []invalidation{{repo: repo, lists: 1}}; !slices.Equal(got, want) {
					t.Errorf("invalidations = %+v, want %+v", got, want)
				}
				if n := len(svc.listed()); n != 2 {
					t.Errorf("listed %d times, want 2", n)
				}
			},
		},
		{
			name: "refresh keeps the selection",
			keys: []string{"down", "down", "r"},
			check: func(t *testing.T, s *host, _ *fakeService, _ []any) {
				t.Helper()
				if pr, _ := s.feed.Selected(); pr.Number != 128 {
					t.Errorf("selected #%d, want #128", pr.Number)
				}
			},
		},
		{
			name: "open in browser opens the selected pull request",
			keys: []string{"down", "o"},
			check: func(t *testing.T, _ *host, _ *fakeService, msgs []any) {
				t.Helper()
				want := ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/pull/135"}
				if !slices.Contains(msgs, any(want)) {
					t.Errorf("messages %v, want %v", msgs, want)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			s := started(t, svc, 80, 12)
			var msgs []any
			for _, k := range tt.keys {
				for _, m := range press(t, s, k) {
					msgs = append(msgs, m)
				}
			}
			tt.check(t, s, svc, msgs)
		})
	}
}

func TestRepoMsg(t *testing.T) {
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	tests := []struct {
		name  string
		setup func(t *testing.T, s *host)
		// want are the repositories listed, in order.
		want []core.RepoRef
	}{
		{
			name: "before Init only stores the repository",
			setup: func(t *testing.T, s *host) {
				t.Helper()
				if cmd := s.Update(ui.RepoMsg{Repo: repo}); cmd != nil {
					t.Error("RepoMsg before Init returned a command")
				}
				drain(t, s, s.Init())
			},
			want: []core.RepoRef{repo},
		},
		{
			name: "after Init lists the new repository",
			setup: func(t *testing.T, s *host) {
				t.Helper()
				drain(t, s, s.Init())
				drain(t, s, s.Update(ui.RepoMsg{Repo: repo}))
				drain(t, s, s.Update(ui.RepoMsg{Repo: other}))
			},
			want: []core.RepoRef{repo, other},
		},
		{
			name: "the same repository again changes nothing",
			setup: func(t *testing.T, s *host) {
				t.Helper()
				drain(t, s, s.Init())
				drain(t, s, s.Update(ui.RepoMsg{Repo: repo}))
				drain(t, s, s.Update(ui.RepoMsg{Repo: repo}))
			},
			want: []core.RepoRef{repo},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			s := newTest(t, svc, 80, 12)
			tt.setup(t, s)
			listed := svc.listed()
			got := make([]core.RepoRef, 0, len(listed))
			for _, q := range listed {
				got = append(got, q.Repo)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("listed %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNoRepoShowsWhatToDo(t *testing.T) {
	s := newTest(t, newFakeService(), 80, 12)
	drain(t, s, s.Init())
	if v := screen(s); !strings.Contains(v, "No repository selected") || !strings.Contains(v, "Press / to search for one.") {
		t.Errorf("screen:\n%s", v)
	}
	if msgs := press(t, s, "f"); len(msgs) != 0 {
		t.Errorf("keys without a repository sent %v", msgs)
	}
	if len(s.Help().ShortHelp()) != 0 {
		t.Error("help offers keys without a repository")
	}
}

func TestNoRepoWithoutSearchKey(t *testing.T) {
	keys := config.Default().Keys
	delete(keys, config.ActionSearch)
	s := New(t.Context(), newFakeService(), keys)
	s.SetSize(30, 8)
	v := ansi.Strip(s.View())
	if !strings.Contains(v, "Search for a") || strings.Contains(v, "Press") {
		t.Errorf("screen without a search key:\n%s", v)
	}
	for i, l := range strings.Split(s.View(), "\n") {
		if w := ansi.StringWidth(l); w != 30 {
			t.Errorf("line %d is %d wide: %q", i, w, ansi.Strip(l))
		}
	}
}

func TestViewFitsSize(t *testing.T) {
	for _, w := range []int{36, 40, 60, 80, 100, 120, 200} {
		s := started(t, newFakeService(), w, 10)
		lines := strings.Split(s.View(), "\n")
		if len(lines) != 10 {
			t.Fatalf("width %d: %d lines, want 10", w, len(lines))
		}
		for i, l := range lines {
			if got := ansi.StringWidth(l); got != w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, got, ansi.Strip(l))
			}
		}
	}
}

func TestColumnsDropLeastImportantFirst(t *testing.T) {
	all := columns{draft: true, review: true, checks: true, diff: true, labels: true, author: true, age: true}
	tests := []struct {
		width int
		want  columns
	}{
		{118, all},
		{98, columns{draft: true, review: true, checks: true, labels: true, author: true, age: true}},
		{78, columns{draft: true, review: true, checks: true, author: true, age: true}},
		{58, columns{review: true, checks: true, author: true, age: true}},
		{38, columns{review: true, checks: true, age: true}},
		{34, columns{review: true, checks: true}},
		{20, columns{}},
	}
	for _, tt := range tests {
		got := columnsFor(tt.width)
		tt.want.width, tt.want.title = tt.width, got.title
		if got != tt.want {
			t.Errorf("columnsFor(%d) = %+v, want %+v", tt.width, got, tt.want)
		}
		if got.title < minTitle && got.checks {
			t.Errorf("columnsFor(%d): title %d is below the minimum", tt.width, got.title)
		}
	}
}

func TestCompact(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1234: "1.2k", 23_456: "23k", 999_999: "999k", 1_500_000: "1M"} {
		if got := compact(n); got != want {
			t.Errorf("compact(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"a longer title", 10, "a longer …"},
		{"naïve café title", 10, "naïve caf…"},
		{"日本語のタイトル", 7, "日本語…"},
		{"x", 0, ""},
	}
	for _, tt := range tests {
		got, w := truncate(tt.in, tt.width)
		if got != tt.want || w != ansi.StringWidth(got) {
			t.Errorf("truncate(%q, %d) = %q, %d; want %q", tt.in, tt.width, got, w, tt.want)
		}
	}
}

func TestRefreshRetriesAFailedPage(t *testing.T) {
	svc := newFakeService()
	svc.listErr = errors.New("502 Bad Gateway")
	s := started(t, svc, 80, 12)
	if !strings.Contains(screen(s), "r to retry") {
		t.Errorf("error row doesn't offer refresh as retry:\n%s", screen(s))
	}
	svc.mu.Lock()
	svc.listErr = nil
	svc.mu.Unlock()
	press(t, s, "r")
	if got := len(svc.invalidations()); got != 1 {
		t.Errorf("retry invalidated %d times, want 1", got)
	}
	if got := s.feed.Len(); got == 0 {
		t.Errorf("no rows after retry:\n%s", screen(s))
	}
}
