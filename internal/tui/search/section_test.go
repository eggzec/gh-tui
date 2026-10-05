package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func screen(s *Section) string { return ansi.Strip(s.View()) }

// flat is the view with styles removed and whitespace collapsed.
func flat(s *Section) string { return strings.Join(strings.Fields(screen(s)), " ") }

func TestDebounce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFake()
		s := newSection(t, svc, 120, 30, WithDebounce(DefaultDebounce))
		// Type a query a key at a time, faster than the debounce.
		waits := make([]tea.Cmd, 0, 3)
		for _, r := range "tea" {
			waits = append(waits, s.Update(keyPress(string(r))))
			time.Sleep(100 * time.Millisecond)
		}
		if n, _ := svc.stats(); n != 0 {
			t.Fatalf("%d searches while typing, want none before the debounce", n)
		}
		for _, w := range waits {
			run(t, s, w)
		}
		if n, _ := svc.stats(); n != 1 || svc.queries[0].Text != "tea" {
			t.Errorf("searched %v, want one search for tea once typing settled", svc.queries)
		}
		if s.Query() != "tea" {
			t.Errorf("the results are for %q, want tea", s.Query())
		}
	})
}

func TestCounts(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	for _, want := range []string{"Repositories 43", "Issues 2", "Pull requests 3", "Code ↵ search", "Repositories · 43 results"} {
		if !strings.Contains(flat(s), want) {
			t.Errorf("the page doesn't show %q:\n%s", want, screen(s))
		}
	}
	// Without a wait, each key settles a query: a search of the kind on
	// view with the counts, and a prefetch of each other kind.
	if n, code := svc.stats(); n != 3 || svc.prefetches != 6 || code != 0 {
		t.Errorf("%d searches, %d prefetches and %d code searches, want 3, 6 and none", n, svc.prefetches, code)
	}
}

func TestSwitchingKindsUsesTheCache(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	before := svc.fetches()
	press(t, s, "tab", "down")
	if s.Kind() != core.SearchIssues || !strings.Contains(screen(s), "Terminal tea renders twice after resize") {
		t.Fatalf("down on the kinds should show the issues:\n%s", screen(s))
	}
	press(t, s, "down")
	if !strings.Contains(screen(s), "render only the cells that changed") {
		t.Errorf("down again should show the pull requests:\n%s", screen(s))
	}
	if n := svc.fetches(); n != before {
		t.Errorf("switching kinds made %d searches, want none", n-before)
	}
}

func TestCodeOnDemand(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	if _, code := svc.stats(); code != 0 {
		t.Fatal("typing searched code")
	}
	// Showing the code searches it.
	press(t, s, "tab", "down", "down", "down")
	if _, code := svc.stats(); code != 1 || s.Kind() != core.SearchCode {
		t.Fatalf("showing the code made %d code searches, want one", code)
	}
	if !strings.Contains(screen(s), "pkg/cmd/tea/tea.go") {
		t.Errorf("the code results are missing:\n%s", screen(s))
	}
	// A new query waits for enter.
	press(t, s, "shift+tab")
	typeText(t, s, "s")
	if _, code := svc.stats(); code != 1 {
		t.Error("typing on the code searched it")
	}
	if !strings.Contains(screen(s), "Press ↵ to search code for “teas”") {
		t.Errorf("the page should say how to search code:\n%s", screen(s))
	}
	press(t, s, "backspace", "enter")
	if _, code := svc.stats(); code != 1 {
		t.Error("enter on a query already searched should use the cache")
	}
	press(t, s, "tab")
	typeText(t, s, "x")
	press(t, s, "enter")
	if _, code := svc.stats(); code != 2 {
		t.Error("enter should search code for the new query")
	}
}

func TestCodeCountdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFake()
		svc.limited = time.Now().Add(42 * time.Second)
		s := newSection(t, svc, 120, 30, WithNow(time.Now))
		typeText(t, s, "tea")
		press(t, s, "tab", "down", "down", "down")
		if _, code := svc.stats(); code != 0 {
			t.Error("code search ran while GitHub is out of them")
		}
		if !strings.Contains(screen(s), "Code search resumes in 42s.") || !strings.Contains(flat(s), "Code in 42s") {
			t.Fatalf("the page should count down:\n%s", screen(s))
		}
		// The tick the page asked for has slept a second, in the bubble.
		run(t, s, s.Update(codeTickMsg{id: s.id}))
		if !strings.Contains(screen(s), "Code search resumes in 41s.") {
			t.Errorf("a second later the countdown should say 41s:\n%s", screen(s))
		}
		time.Sleep(time.Until(svc.limited))
		run(t, s, s.Update(codeTickMsg{id: s.id}))
		if s.limited() || !strings.Contains(screen(s), "Press ↵ to search code") {
			t.Errorf("once code search resumes the page should offer it:\n%s", screen(s))
		}
		press(t, s, "enter")
		if _, code := svc.stats(); code != 1 {
			t.Error("enter should search code once it resumes")
		}
	})
}

func TestCodeRefused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFake()
		svc.codeErr = &core.RateLimitError{Reset: time.Now().Add(30 * time.Second)}
		s := newSection(t, svc, 120, 30, WithNow(time.Now))
		typeText(t, s, "tea")
		press(t, s, "tab", "down", "down", "down")
		if !strings.Contains(screen(s), "Code search resumes in 30s.") {
			t.Errorf("a refused code search should count down to when GitHub said:\n%s", screen(s))
		}
	})
}

// A failed code search says what went wrong the way the user should read
// it, without the error's chain, request or status code.
func TestCodeErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"offline", fmt.Errorf("search code: github: GET /search/code: %w", core.ErrOffline), errMark + " Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("search code: github: 403 Forbidden: %w", core.ErrForbidden), errMark + " You don't have access to this · o to open on GitHub"},
		{"internal", errors.New("search code: github: decode: unexpected EOF"), errMark + " Something went wrong · r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFake()
			svc.codeErr = tt.err
			s := newSection(t, svc, 120, 30)
			typeText(t, s, "tea")
			press(t, s, "tab", "down", "down", "down", "enter")
			if v := screen(s); !strings.Contains(v, tt.want) || strings.Contains(v, "github:") || strings.Contains(v, "403") {
				t.Errorf("screen = %q, want %q", v, tt.want)
			}
		})
	}
}

func TestInvalidQuery(t *testing.T) {
	s := newSection(t, newFake(), 120, 30)
	typeText(t, s, "tea is:bogus")
	if view := screen(s); !strings.Contains(view, "GitHub can't run this query: is:bogus is not a valid qualifier") {
		t.Errorf("an invalid query should say why, inline:\n%s", view)
	}
}

func TestPaging(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	press(t, s, "down", "end")
	if n := len(svc.queries); n < 2 {
		t.Fatalf("scrolling to the end made %d searches, want the next page", n)
	}
	next := svc.queries[len(svc.queries)-1]
	if next.Kind != core.SearchRepos || next.Cursor == "" {
		t.Errorf("the next page asked %+v, want the repositories after the first page", next)
	}
	if l, _ := s.visibleHits(); l.feed.Len() < 40 {
		t.Errorf("the list holds %d repositories, want more pages", l.feed.Len())
	}
}

func TestOpen(t *testing.T) {
	bubbletea := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	tests := []struct {
		name string
		keys []string
		want []tea.Msg
	}{
		{"repository", []string{"down", "enter"}, []tea.Msg{ui.RepoMsg{Repo: bubbletea}}},
		{"repository in the browser", []string{"down", "o"}, []tea.Msg{ui.OpenMsg{URL: "https://github.com/charmbracelet/bubbletea"}}},
		{"issue", []string{"tab", "down", "enter", "enter"}, []tea.Msg{ui.OpenIssueMsg{Repo: bubbletea, Number: 1203}}},
		{"pull request", []string{"tab", "down", "down", "enter", "down", "enter"}, []tea.Msg{ui.OpenPullMsg{Repo: bubbletea, Number: 1388}}},
		{"pull request on its checks", []string{"tab", "down", "down", "enter", "down", "C"}, []tea.Msg{ui.OpenPullMsg{Repo: bubbletea, Number: 1388, Checks: true}}},
		{"no checks for an issue", []string{"tab", "down", "enter", "C"}, nil},
		{"issue's repository", []string{"tab", "down", "enter", "ctrl+o"}, []tea.Msg{ui.RepoMsg{Repo: bubbletea}}},
		{"pull request's repository", []string{"tab", "down", "down", "enter", "down", "ctrl+o"}, []tea.Msg{ui.RepoMsg{Repo: bubbletea}}},
		{"repository by the repository key", []string{"down", "ctrl+o"}, []tea.Msg{ui.RepoMsg{Repo: bubbletea}}},
		// The file opens over the page, on what matched.
		{"file", []string{"tab", "down", "down", "down", "enter", "enter"}, []tea.Msg{ui.OpenFileMsg{Repo: bubbletea, Path: "tea.go", SHA: "b1", Find: "tea"}}},
		{"file's repository", []string{"tab", "down", "down", "down", "enter", "ctrl+o"}, []tea.Msg{ui.RepoMsg{Repo: bubbletea}}},
		{"file in the browser", []string{"tab", "down", "down", "down", "enter", "o"}, []tea.Msg{ui.OpenMsg{URL: "https://github.com/charmbracelet/bubbletea/blob/main/tea.go"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), 120, 30)
			typeText(t, s, "tea")
			if got := press(t, s, tt.keys...); !slices.Equal(got, tt.want) {
				t.Errorf("sent %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFirstMatch(t *testing.T) {
	for _, tt := range []struct {
		name string
		hit  core.CodeHit
		want string
	}{
		{"none", core.CodeHit{}, ""},
		{"first", core.CodeHit{Fragments: []core.Fragment{{Text: "a Foo b bar", Matches: [][2]int{{2, 5}, {8, 11}}}}}, "Foo"},
		{"one line", core.CodeHit{Fragments: []core.Fragment{{Text: "x foo\nbar", Matches: [][2]int{{2, 9}}}}}, "foo"},
		{"skips blank and out of range", core.CodeHit{Fragments: []core.Fragment{
			{Text: "  ", Matches: [][2]int{{0, 2}, {1, 9}}},
			{Text: "then", Matches: [][2]int{{0, 4}}},
		}}, "then"},
	} {
		if got := firstMatch(tt.hit); got != tt.want {
			t.Errorf("%s: firstMatch = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestBack(t *testing.T) {
	s := newSection(t, newFake(), 120, 30)
	for _, keys := range [][]string{{"esc"}, {"tab", "esc"}, {"down", "esc"}} {
		s.Focus()
		if got := press(t, s, keys...); !slices.Equal(got, []tea.Msg{ui.BackMsg{}}) {
			t.Errorf("%v sent %v, want to go back", keys, got)
		}
	}
}

func TestCapturing(t *testing.T) {
	s := newSection(t, newFake(), 120, 30)
	if !s.Capturing() {
		t.Error("the query should take every key")
	}
	typeText(t, s, "q]")
	if s.input.Value() != "q]" {
		t.Errorf("the query is %q, want the keys typed", s.input.Value())
	}
	press(t, s, "down")
	if s.Capturing() {
		t.Error("the results take only their own keys")
	}
}

func TestStart(t *testing.T) {
	s := newSection(t, newFake(), 120, 30)
	if view := screen(s); !strings.Contains(view, "Your repositories") || !strings.Contains(view, "octocat/hello-world") {
		t.Fatalf("an empty query should offer the repositories:\n%s", view)
	}
	app := press(t, s, "down", "enter")
	if !slices.Equal(app, []tea.Msg{ui.RepoMsg{Repo: core.RepoRef{Owner: "octocat", Name: "hello-world"}}}) {
		t.Errorf("enter on a repository sent %v", app)
	}
	// A search opened is remembered, and picking it searches again.
	s.Focus()
	typeText(t, s, "lipgloss")
	press(t, s, "down", "enter")
	s.Focus()
	for range len("lipgloss") {
		press(t, s, "backspace")
	}
	if view := screen(s); !strings.Contains(view, "Recent searches") || !strings.Contains(view, "↺ lipgloss") {
		t.Fatalf("the search should be remembered:\n%s", view)
	}
	press(t, s, "down", "enter")
	if s.Query() != "lipgloss" || s.input.Value() != "lipgloss" {
		t.Errorf("picking a recent search should search it again, got %q", s.Query())
	}
}

// The repositories offered before the user types say why they failed to
// load the way the user should read it, without the error's chain, request
// or status code. They load once, and the open key opens a recent search,
// so no hint names a key.
func TestStartFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"offline", fmt.Errorf("list repos: github: GET /user/repos: %w", core.ErrOffline), errMark + " Can't reach GitHub"},
		{"forbidden", fmt.Errorf("list repos: github: 403 Forbidden: %w", core.ErrForbidden), errMark + " You don't have access to this"},
		{"not found", fmt.Errorf("list repos: github: 404 Not Found: %w", core.ErrNotFound), errMark + " This doesn't exist or is private."},
		{"internal", fmt.Errorf("list repos: github: decode: %s", termtexttest.Hostile), errMark + " Something went wrong"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), 120, 30, WithStart(func(context.Context) ([]core.Repo, error) { return nil, tt.err }))
			termtexttest.AssertClean(t, s.View(), 120)
			view := screen(s)
			if !strings.Contains(view, tt.want) || !strings.Contains(view, "Type to search GitHub.") {
				t.Errorf("the page should say %q:\n%s", tt.want, view)
			}
			for _, leak := range []string{"github", "list repos", "GET", "403", "404", "decode", " · "} {
				if strings.Contains(view, leak) {
					t.Errorf("the page shows %q:\n%s", leak, view)
				}
			}
		})
	}
}

func TestRefresh(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	before, _ := svc.stats()
	press(t, s, "down", "r")
	if n, _ := svc.stats(); svc.invalidated != 1 || n != before+1 {
		t.Errorf("refresh invalidated %d times and searched %d times, want once each", svc.invalidated, n-before)
	}
}

func TestResultsOfAnotherQueryAreDropped(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30, WithDebounce(time.Hour))
	// The wait isn't run, so it never ends.
	s.Update(keyPress("t"))
	stale := debounceMsg{id: s.id, seq: s.seq - 1}
	s.Update(stale)
	if s.Query() != "" {
		t.Error("a wait for an earlier edit should be dropped")
	}
	s.Update(debounceMsg{id: s.id + 1, seq: s.seq})
	if s.Query() != "" {
		t.Error("a wait of another page should be dropped")
	}
}

func TestKeepsResultsWhileLoading(t *testing.T) {
	s := newSection(t, newFake(), 120, 30)
	// The first query has nothing to keep, so it shows the loading body.
	s.Update(keyPress("t"))
	if view := screen(s); !strings.Contains(view, "Loading…") {
		t.Errorf("a first query should show the loading body:\n%s", view)
	}
	typeText(t, s, "ea")
	// A new query keeps the results of the one before, dimmed, until its
	// own arrive, with a spinner in the title.
	pending := s.Update(keyPress("-"))
	view := screen(s)
	if strings.Contains(view, "Loading…") || !strings.Contains(view, "charmbracelet/bubbletea") {
		t.Errorf("the results of tea should stay while tea- loads:\n%s", view)
	}
	if !strings.Contains(view, "Repositories "+ansi.Strip(s.spin.View())) {
		t.Errorf("the title should spin while tea- loads:\n%s", view)
	}
	run(t, s, pending)
	view = screen(s)
	if !strings.Contains(view, "octo/tea-00") || strings.Contains(view, "charmbracelet/bubbletea") {
		t.Errorf("the results of tea- should replace those of tea:\n%s", view)
	}
	if _, ok := s.staleView(); ok {
		t.Error("the dimmed results should be gone")
	}
}

func TestNewTextCancelsReads(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	svc.mu.Lock()
	old := slices.Clone(svc.ctxs)
	svc.mu.Unlock()
	if len(old) != 9 {
		t.Fatalf("%d reads, want a search and two prefetches per key", len(old))
	}
	typeText(t, s, "s")
	for i, ctx := range old {
		if ctx.Err() == nil {
			t.Errorf("read %d for an earlier text still runs", i)
		}
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if ctx := svc.ctxs[len(svc.ctxs)-1]; ctx.Err() != nil {
		t.Error("the reads for the text on view were canceled")
	}
}

// On an Enterprise host, a repository without a URL links to its pages.
func TestRepoURLOnHost(t *testing.T) {
	s := &Section{host: "ghe.example.com"}
	got := s.hitURL(core.SearchHit{Kind: core.SearchRepos, Repo: core.Repo{Ref: core.RepoRef{Owner: "o", Name: "r"}}})
	if want := "https://ghe.example.com/o/r"; got != want {
		t.Errorf("hitURL = %q, want %q", got, want)
	}
}

func TestSearch(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	run(t, s, s.Search("  tea  "))
	if s.Query() != "tea" || s.input.Value() != "  tea  " {
		t.Errorf("results for %q with %q typed, want tea as typed", s.Query(), s.input.Value())
	}
	if s.Capturing() || !strings.Contains(screen(s), "charmbracelet/bubbletea") {
		t.Errorf("the results should have the focus, as after enter:\n%s", screen(s))
	}
	if !slices.Contains(s.recent, "tea") {
		t.Error("the search wasn't remembered, as enter remembers it")
	}
}

func TestFreshStartsEmptyAndFocused(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	for _, k := range []string{"t", "e", "a"} {
		run(t, s, s.Update(keyPress(k)))
	}
	run(t, s, s.Update(keyPress("enter")))
	if s.Query() != "tea" || s.area != resultsArea || s.Capturing() {
		t.Fatalf("after enter: query %q, area %d, capturing %v; want tea in the results", s.Query(), s.area, s.Capturing())
	}

	s.Fresh()
	if s.input.Value() != "" || s.Query() != "" {
		t.Errorf("input %q, query %q after Fresh, want both empty", s.input.Value(), s.Query())
	}
	if !s.Capturing() || !s.input.Focused() {
		t.Errorf("the query isn't focused after Fresh")
	}
	for _, k := range []string{"g", "o"} {
		run(t, s, s.Update(keyPress(k)))
	}
	if got := s.input.Value(); got != "go" {
		t.Errorf("typed go after Fresh, the input is %q", got)
	}
}

func TestFreshReadsTheSameQueryAnew(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	// The commands of the first typing are dropped, as when the read of
	// its first page is still on its way as the user leaves.
	for _, r := range "tea" {
		s.Update(keyPress(string(r)))
	}
	if svc.requests != 0 {
		t.Fatalf("%d requests before any command ran", svc.requests)
	}
	s.Fresh()
	// Search sets the whole text at once, so the list of the query
	// before, which is for the same text, is the one it would reuse.
	run(t, s, s.Search("tea"))
	if svc.requests == 0 {
		t.Fatal("typing the same query again read nothing")
	}
	l, ok := s.visibleHits()
	if !ok || l.feed.Len() == 0 || l.feed.Err() != nil {
		t.Errorf("the list of the query typed again has %d results, error %v, want results", l.feed.Len(), l.feed.Err())
	}
}

func TestFreshDropsTheDebounceInFlight(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30, WithDebounce(time.Second))
	for _, r := range "tea" {
		s.Update(keyPress(string(r)))
	}
	stale := debounceMsg{id: s.id, seq: s.seq}
	s.Fresh()
	run(t, s, s.Update(stale))
	if s.Query() != "" || s.input.Value() != "" {
		t.Errorf("query %q, input %q after the stale debounce, want both empty", s.Query(), s.input.Value())
	}
	if _, ok := s.visibleHits(); ok {
		t.Error("the stale debounce made a list of results")
	}
	if svc.requests != 0 {
		t.Errorf("%d requests after the stale debounce, want none", svc.requests)
	}
}
