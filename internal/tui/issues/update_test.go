package issues

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

func TestRepoMsg(t *testing.T) {
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	tests := []struct {
		name string
		do   func(t *testing.T, s *host, svc *fakeService)
		// wantRepos are the repositories listed, in order.
		wantRepos []core.RepoRef
		wantRows  int
	}{
		{
			name: "before init waits for init",
			do: func(t *testing.T, s *host, _ *fakeService) {
				t.Helper()
				if cmd := s.Update(ui.RepoMsg{Repo: testRepo}); cmd != nil {
					t.Error("RepoMsg before Init returned a command")
				}
				run(t, s, s.Init())
			},
			wantRepos: []core.RepoRef{testRepo},
			wantRows:  10,
		},
		{
			name: "after init loads at once",
			do: func(t *testing.T, s *host, _ *fakeService) {
				t.Helper()
				if cmd := s.Init(); cmd != nil {
					t.Error("Init without a repository returned a command")
				}
				run(t, s, s.Update(ui.RepoMsg{Repo: testRepo}))
			},
			wantRepos: []core.RepoRef{testRepo},
			wantRows:  10,
		},
		{
			name: "a new repository resets the list",
			do: func(t *testing.T, s *host, _ *fakeService) {
				t.Helper()
				run(t, s, s.Update(ui.RepoMsg{Repo: testRepo}))
				run(t, s, s.Init())
				press(t, s, "down", "down")
				run(t, s, s.Update(ui.RepoMsg{Repo: other}))
			},
			wantRepos: []core.RepoRef{testRepo, other},
		},
		{
			name: "the same repository is kept",
			do: func(t *testing.T, s *host, _ *fakeService) {
				t.Helper()
				run(t, s, s.Update(ui.RepoMsg{Repo: testRepo}))
				run(t, s, s.Init())
				if cmd := s.Update(ui.RepoMsg{Repo: testRepo}); cmd != nil {
					t.Error("the same repository returned a command")
				}
			},
			wantRepos: []core.RepoRef{testRepo},
			wantRows:  10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			s := newSection(t, svc, 80, 20)
			tt.do(t, s, svc)
			calls := svc.listCalls()
			repos := make([]core.RepoRef, 0, len(calls))
			for _, q := range calls {
				repos = append(repos, q.Repo)
			}
			if !slices.Equal(repos, tt.wantRepos) {
				t.Errorf("listed %v, want %v", repos, tt.wantRepos)
			}
			if got := s.list.Len(); got != tt.wantRows {
				t.Errorf("list has %d rows, want %d", got, tt.wantRows)
			}
		})
	}
}

func TestEmptyStateWithoutRepo(t *testing.T) {
	s := newSection(t, newFakeService(nil), 80, 10)
	run(t, s, s.Init())
	v := ansi.Strip(s.View())
	if !strings.Contains(v, "No repository selected") || !strings.Contains(v, "Press / to search for one.") {
		t.Errorf("view without a repository = %q", v)
	}
	if msgs := press(t, s, "f", "r", "enter", "o"); len(msgs) != 0 {
		t.Errorf("keys without a repository sent %v", msgs)
	}
	if len(uitest.Enabled(s.KeyLayers())) != 0 {
		t.Error("help lists keys without a repository")
	}
}

func TestEmptyStateWithoutSearchKey(t *testing.T) {
	keys := config.Default().Keys
	delete(keys, config.ActionSearch)
	s := New(t.Context(), newFakeService(nil), keys)
	s.SetSize(30, 8)
	v := s.View()
	assertFits(t, v, 30, 8)
	if v := ansi.Strip(v); !strings.Contains(v, "Search for a") || strings.Contains(v, "Press") {
		t.Errorf("view without a search key:\n%s", v)
	}
}

func TestFilter(t *testing.T) {
	tests := []struct {
		presses int
		want    core.StateFilter
		rows    int
	}{
		{0, core.FilterOpen, 16},
		{1, core.FilterClosed, 4},
		{2, core.FilterAll, 20},
		{3, core.FilterOpen, 16},
	}
	for _, tt := range tests {
		t.Run(string(tt.want), func(t *testing.T) {
			svc := newFakeService(sampleIssues(20))
			s := started(t, svc, 80, 30)
			press(t, s, "down")
			for range tt.presses {
				press(t, s, "]")
			}
			calls := svc.listCalls()
			if got := calls[len(calls)-1].State; got != tt.want {
				t.Errorf("last list filter = %q, want %q", got, tt.want)
			}
			if got := s.list.Len(); got != tt.rows {
				t.Errorf("rows = %d, want %d", got, tt.rows)
			}
			if tt.presses > 0 && s.list.Index() != 0 {
				t.Errorf("a new filter kept the selection at %d", s.list.Index())
			}
			bar := ansi.Strip(strings.SplitN(s.View(), "\n", 2)[0])
			if !strings.Contains(bar, "Open · Closed · All") {
				t.Errorf("bar = %q, want the tabs", bar)
			}
		})
	}
}

func TestRefresh(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := started(t, svc, 80, 20)
	press(t, s, "down", "down")
	svc.set(998, func(it *core.Issue) { it.Title = "Renamed on the server" })
	before := len(svc.listCalls())
	press(t, s, "r")
	if got, want := svc.invalidations(), []invalidation{{repo: testRepo, lists: before}}; !slices.Equal(got, want) {
		t.Errorf("invalidations = %+v, want %+v", got, want)
	}
	if got := len(svc.listCalls()) - before; got != 1 {
		t.Fatalf("refresh listed %d pages, want 1", got)
	}
	it, _ := s.list.Selected()
	if it.Number != 998 || it.Title != "Renamed on the server" {
		t.Errorf("selected after refresh = #%d %q, want #998 renamed", it.Number, it.Title)
	}
}

func TestRefreshRetriesAFailedPage(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.listErr = errors.New("502 Bad Gateway")
	s := started(t, svc, 80, 10)
	if !strings.Contains(ansi.Strip(s.View()), "r to retry") {
		t.Errorf("error row doesn't offer refresh as retry:\n%s", ansi.Strip(s.View()))
	}
	svc.listErr = nil
	press(t, s, "r")
	if got := len(svc.invalidations()); got != 1 {
		t.Errorf("retry invalidated %d times, want 1", got)
	}
	if got := s.list.Len(); got != 10 {
		t.Errorf("rows after retry = %d, want 10", got)
	}
}

func TestOpenInBrowser(t *testing.T) {
	s := started(t, newFakeService(sampleIssues(12)), 80, 20)
	msgs := press(t, s, "down", "o")
	open, ok := has[ui.OpenMsg](msgs)
	if !ok || open.URL != "https://github.com/eggzec/gh-tui/issues/999" {
		t.Errorf("o sent %v, want to open issue 999", msgs)
	}
}

func TestListQueryLeavesPageSizeToService(t *testing.T) {
	svc := newFakeService(sampleIssues(80))
	s := started(t, svc, 80, 40)
	press(t, s, "G")
	for _, q := range svc.listCalls() {
		if q.PageSize != 0 {
			t.Fatalf("list query %+v sets a page size", q)
		}
	}
	calls := svc.listCalls()
	if !slices.ContainsFunc(calls, func(q issuesvc.ListQuery) bool { return q.Cursor == "30" }) {
		t.Errorf("list queries %v never asked for the second page", calls)
	}
}

// The list says what went wrong the way the user should read it, without
// the error's chain, request or status code.
func TestErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"offline", fmt.Errorf("list issues: github: GET /repos/o/r/issues: %w", core.ErrOffline), "✗ Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("list issues: github: 403 Forbidden: %w", core.ErrForbidden), "✗ You don't have access to eggzec/gh-tui · o to open on GitHub"},
		{"internal", errors.New("list issues: github: decode: unexpected EOF"), "✗ Something went wrong · r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			svc.listErr = tt.err
			s := started(t, svc, 100, 10)
			if v := ansi.Strip(s.View()); !strings.Contains(v, tt.want) || strings.Contains(v, "github") || strings.Contains(v, "403") {
				t.Errorf("screen = %q, want %q", v, tt.want)
			}
		})
	}
}
