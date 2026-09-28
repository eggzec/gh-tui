package search

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

// fakeAPI answers with its func fields and records the calls. A nil field
// fails the test when it is called.
type fakeAPI struct {
	t *testing.T

	search func(q github.SearchQuery) (map[core.SearchKind]core.SearchPage[core.SearchHit], error)
	code   func(query, cursor string, perPage int) (core.SearchPage[core.CodeHit], error)

	mu    sync.Mutex
	calls []string
}

func (f *fakeAPI) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeAPI) Search(_ context.Context, q github.SearchQuery) (map[core.SearchKind]core.SearchPage[core.SearchHit], error) {
	after := slices.Sorted(maps.Keys(q.After))
	parts := make([]string, len(after))
	for i, k := range after {
		parts[i] = string(k) + "=" + q.After[k]
	}
	counted := slices.Sorted(slices.Values(q.Count))
	call := fmt.Sprintf("search %q %d %v", q.Text, q.First, parts)
	if len(counted) > 0 {
		call += fmt.Sprintf(" count %v", counted)
	}
	f.record(call)
	if f.search == nil {
		f.t.Error("unexpected Search")
		return nil, errors.New("unexpected call")
	}
	return f.search(q)
}

func (f *fakeAPI) SearchCode(_ context.Context, query, cursor string, perPage int) (core.SearchPage[core.CodeHit], error) {
	f.record(fmt.Sprintf("code %q %d %s", query, perPage, cursor))
	if f.code == nil {
		f.t.Error("unexpected SearchCode")
		return core.SearchPage[core.CodeHit]{}, errors.New("unexpected call")
	}
	return f.code(query, cursor, perPage)
}

func (f *fakeAPI) wantCalls(t *testing.T, want ...string) {
	t.Helper()
	f.mu.Lock()
	got := slices.Clone(f.calls)
	f.mu.Unlock()
	if !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

var (
	ghTUI = core.Repo{ID: "R_1", Ref: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}}
	crash = core.SearchHit{Kind: core.SearchIssues, Issue: core.Issue{ID: "I_1", Repo: ghTUI.Ref, Number: 42, Title: "Crash"}}
	fixPR = core.SearchHit{Kind: core.SearchPulls, Issue: core.Issue{ID: "PR_1", Repo: ghTUI.Ref, Number: 7, Title: "Fix crash"}}
	fixP2 = core.SearchHit{Kind: core.SearchPulls, Issue: core.Issue{ID: "PR_2", Repo: ghTUI.Ref, Number: 8, Title: "Fix more"}}
	repo  = core.SearchHit{Kind: core.SearchRepos, Repo: ghTUI}
)

func page(total int, next string, hits ...core.SearchHit) core.SearchPage[core.SearchHit] {
	return core.SearchPage[core.SearchHit]{Items: hits, Next: next, Total: total}
}

// answer returns the first page of each kind asked for, or of every kind,
// the count alone of each kind counted, or the second page of pull
// requests.
func answer(q github.SearchQuery) (map[core.SearchKind]core.SearchPage[core.SearchHit], error) {
	first := map[core.SearchKind]core.SearchPage[core.SearchHit]{
		core.SearchRepos:  page(1, "", repo),
		core.SearchIssues: page(5, "i2", crash),
		core.SearchPulls:  page(12, "p2", fixPR),
	}
	if len(q.After) == 0 && len(q.Count) == 0 {
		return first, nil
	}
	if q.After[core.SearchPulls] == "p2" {
		return map[core.SearchKind]core.SearchPage[core.SearchHit]{core.SearchPulls: page(13, "", fixP2)}, nil
	}
	out := make(map[core.SearchKind]core.SearchPage[core.SearchHit], 3)
	for kind, cursor := range q.After {
		if cursor != "" {
			return nil, fmt.Errorf("unexpected query %+v", q)
		}
		out[kind] = first[kind]
	}
	for _, kind := range q.Count {
		out[kind] = page(first[kind].Total, "")
	}
	return out, nil
}

func ids(p core.Page[core.SearchHit]) []string {
	out := make([]string, len(p.Items))
	for i := range p.Items {
		h := &p.Items[i]
		if h.Kind == core.SearchRepos {
			out[i] = h.Repo.ID
		} else {
			out[i] = h.Issue.ID
		}
	}
	return out
}

var allCounts = map[core.SearchKind]int{core.SearchRepos: 1, core.SearchIssues: 5, core.SearchPulls: 12}

// The first page of one kind counts every kind, and the first pages of the
// others are prefetched apart, so switching kinds costs nothing.
func TestSearchFirstPageCountsEveryKind(t *testing.T) {
	api := &fakeAPI{t: t, search: answer}
	s := New(api)
	got, err := s.Search(t.Context(), Query{Text: "  fix   crash ", Kind: core.SearchIssues})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !slices.Equal(ids(got.Page), []string{"I_1"}) || got.Total != 5 || got.Next != "i2" {
		t.Errorf("issues = %v total %d next %q", ids(got.Page), got.Total, got.Next)
	}
	if !maps.Equal(got.Counts, allCounts) {
		t.Errorf("Counts = %v, want %v", got.Counts, allCounts)
	}
	for _, tt := range []struct {
		kind core.SearchKind
		want string
	}{{core.SearchRepos, "R_1"}, {core.SearchPulls, "PR_1"}} {
		kind, want := tt.kind, tt.want
		q := Query{Text: "fix crash", Kind: kind}
		// A count isn't a page.
		if _, ok := s.CachedSearch(q); ok {
			t.Errorf("CachedSearch(%s) hit before its prefetch", kind)
		}
		if err := s.Prefetch(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		c, ok := s.CachedSearch(q)
		if !ok || !slices.Equal(ids(c.Page), []string{want}) || !maps.Equal(c.Counts, allCounts) {
			t.Errorf("CachedSearch(%s) = %v %v, %t", kind, ids(c.Page), c.Counts, ok)
		}
		if _, err := s.Search(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		if err := s.Prefetch(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	api.wantCalls(t,
		`search "fix crash" 20 [issues=] count [pulls repos]`,
		`search "fix crash" 20 [repos=]`,
		`search "fix crash" 20 [pulls=]`,
	)
	if err := s.Prefetch(t.Context(), Query{Text: "x"}); err == nil {
		t.Error("a prefetch of every kind succeeded")
	}
}

func TestSearchNextPageAsksForOneKind(t *testing.T) {
	api := &fakeAPI{t: t, search: answer}
	s := New(api)
	first, err := s.Search(t.Context(), Query{Text: "fix", Kind: core.SearchPulls})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Search(t.Context(), Query{Text: "fix", Kind: core.SearchPulls, Cursor: first.Next})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(second.Page), []string{"PR_2"}) || !second.Last() {
		t.Errorf("second page = %v next %q, want PR_2 on the last page", ids(second.Page), second.Next)
	}
	// The newer count replaces the pull requests' only.
	want := map[core.SearchKind]int{core.SearchRepos: 1, core.SearchIssues: 5, core.SearchPulls: 13}
	if !maps.Equal(second.Counts, want) {
		t.Errorf("Counts = %v, want %v", second.Counts, want)
	}
	// The first page is still cached as it was.
	if c, ok := s.CachedSearch(Query{Text: "fix", Kind: core.SearchPulls}); !ok || !slices.Equal(ids(c.Page), []string{"PR_1"}) {
		t.Errorf("CachedSearch of the first page = %v, %t", ids(c.Page), ok)
	}
	api.wantCalls(t, `search "fix" 20 [pulls=] count [issues repos]`, `search "fix" 20 [pulls=p2]`)
}

func TestSearchAll(t *testing.T) {
	api := &fakeAPI{t: t, search: answer}
	s := New(api)
	if _, ok := s.CachedSearch(Query{Text: "crash"}); ok {
		t.Error("CachedSearch hit before any search")
	}
	got, err := s.Search(t.Context(), Query{Text: "crash", PageSize: 500})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !slices.Equal(ids(got.Page), []string{"R_1", "I_1", "PR_1"}) || !got.Last() || got.Total != 18 {
		t.Errorf("Search = %v next %q total %d; want every kind on one page", ids(got.Page), got.Next, got.Total)
	}
	if c, ok := s.CachedSearch(Query{Text: "crash", PageSize: 100}); !ok || len(c.Items) != 3 {
		t.Errorf("CachedSearch = %v, %t", ids(c.Page), ok)
	}
	if _, err := s.Search(t.Context(), Query{Text: "crash", Cursor: "x"}); err == nil {
		t.Error("a second page of every kind succeeded")
	}
	api.wantCalls(t, `search "crash" 100 []`)
}

func TestSearchCachesByText(t *testing.T) {
	api := &fakeAPI{t: t, search: answer}
	s := New(api)
	for _, text := range []string{"TUI", "tui", " tui "} {
		if _, err := s.Search(t.Context(), Query{Text: text, Kind: core.SearchRepos}); err != nil {
			t.Fatal(err)
		}
	}
	// Page sizes are cached apart, since their cursors differ.
	if _, ok := s.CachedSearch(Query{Text: "tui", Kind: core.SearchRepos, PageSize: 5}); ok {
		t.Error("CachedSearch hit a page of another size")
	}
	api.wantCalls(t, `search "TUI" 20 [repos=] count [issues pulls]`)
}

func TestSearchEmptyText(t *testing.T) {
	api := &fakeAPI{t: t}
	s := New(api)
	for _, kind := range []core.SearchKind{core.SearchAll, core.SearchRepos} {
		got, err := s.Search(t.Context(), Query{Text: "   ", Kind: kind})
		if err != nil || len(got.Items) != 0 || !got.Last() {
			t.Errorf("Search = %+v, %v; want an empty last page", got, err)
		}
	}
	if got, ok := s.CachedSearch(Query{}); !ok || len(got.Items) != 0 {
		t.Errorf("CachedSearch = %+v, %v; want a known empty page", got, ok)
	}
	if got, err := s.Code(t.Context(), CodeQuery{Text: " "}); err != nil || len(got.Items) != 0 {
		t.Errorf("Code = %+v, %v; want an empty page", got, err)
	}
	if _, ok := s.CachedCode(CodeQuery{}); !ok {
		t.Error("CachedCode of no text missed")
	}
	api.wantCalls(t)
}

func TestSearchErrors(t *testing.T) {
	limited := fmt.Errorf("graphql: %w", &core.RateLimitError{Reset: time.Unix(1790000000, 0)})
	api := &fakeAPI{t: t, search: func(github.SearchQuery) (map[core.SearchKind]core.SearchPage[core.SearchHit], error) {
		return nil, limited
	}}
	s := New(api)
	for _, kind := range []core.SearchKind{core.SearchRepos, core.SearchAll} {
		_, err := s.Search(t.Context(), Query{Text: "tui", Kind: kind})
		if rl, ok := errors.AsType[*core.RateLimitError](err); !ok || !rl.Reset.Equal(time.Unix(1790000000, 0)) {
			t.Fatalf("%q: error = %v, want the rate limit", kind, err)
		}
		if _, ok := s.CachedSearch(Query{Text: "tui", Kind: kind}); ok {
			t.Errorf("%q: a failed search was cached", kind)
		}
	}
	if _, err := s.Search(t.Context(), Query{Text: "tui", Kind: core.SearchCode}); err == nil {
		t.Error("Search of code succeeded; code has its own method")
	}
}

func TestSearchExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := 0
		api := &fakeAPI{t: t, search: func(github.SearchQuery) (map[core.SearchKind]core.SearchPage[core.SearchHit], error) {
			n++
			r := repo
			r.Repo.Stars = n
			return map[core.SearchKind]core.SearchPage[core.SearchHit]{
				core.SearchRepos: page(n, "", r), core.SearchIssues: page(0, ""), core.SearchPulls: page(0, ""),
			}, nil
		}}
		s := New(api)
		q := Query{Text: "tui", Kind: core.SearchRepos}
		if _, err := s.Search(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		time.Sleep(DefaultTTL)
		if got, ok := s.CachedSearch(q); !ok || got.Items[0].Repo.Stars != 1 {
			t.Errorf("stale CachedSearch = %+v, %v; want the old page", got, ok)
		}
		got, err := s.Search(t.Context(), q)
		if err != nil || got.Items[0].Repo.Stars != 2 || got.Counts[core.SearchRepos] != 2 {
			t.Errorf("Search = %+v, %v; want the refetched page and count", got, err)
		}

		s.Invalidate()
		if got, err = s.Search(t.Context(), q); err != nil || got.Items[0].Repo.Stars != 3 {
			t.Errorf("Search after Invalidate = %+v, %v; want a new fetch", got, err)
		}
	})
}

func TestOptions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{t: t, search: answer}
		// Every search stores one page, so a capacity of one keeps one
		// search.
		s := New(api, WithTTL(time.Hour), WithCapacity(1))
		for _, text := range []string{"a", "b", "a"} {
			if _, err := s.Search(t.Context(), Query{Text: text, Kind: core.SearchRepos}); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(time.Minute)
		if _, err := s.Search(t.Context(), Query{Text: "a", Kind: core.SearchRepos}); err != nil {
			t.Fatal(err)
		}
		c := `search "%s" 20 [repos=] count [issues pulls]`
		api.wantCalls(t, fmt.Sprintf(c, "a"), fmt.Sprintf(c, "b"), fmt.Sprintf(c, "a"))
	})
}

var styleGo = core.CodeHit{
	Repo: core.RepoRef{Owner: "charmbracelet", Name: "lipgloss"},
	Path: "style.go",
	Fragments: []core.Fragment{
		{Text: "func NewStyle() Style", Matches: [][2]int{{5, 13}}},
	},
}

func codePage(total int, next string, hits ...core.CodeHit) core.SearchPage[core.CodeHit] {
	return core.SearchPage[core.CodeHit]{Items: hits, Next: next, Total: total}
}

func TestCode(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{
			t:      t,
			search: answer,
			code: func(_, cursor string, _ int) (core.SearchPage[core.CodeHit], error) {
				if cursor == "" {
					return codePage(48, "next", styleGo), nil
				}
				return codePage(48, ""), nil
			},
		}
		s := New(api)
		q := CodeQuery{Text: "NewStyle"}
		if _, ok := s.CachedCode(q); ok {
			t.Error("CachedCode hit before any search")
		}
		got, err := s.Code(t.Context(), q)
		if err != nil || got.Total != 48 || len(got.Items) != 1 || got.Next != "next" {
			t.Fatalf("Code = %+v, %v", got, err)
		}
		if _, err := s.Code(t.Context(), CodeQuery{Text: "newstyle", PageSize: DefaultPageSize}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Code(t.Context(), CodeQuery{Text: "NewStyle", Cursor: got.Next}); err != nil {
			t.Fatal(err)
		}
		// The code count joins the others.
		res, err := s.Search(t.Context(), Query{Text: "NewStyle", Kind: core.SearchRepos})
		if err != nil || res.Counts[core.SearchCode] != 48 || res.Counts[core.SearchRepos] != 1 {
			t.Errorf("Counts = %v, %v; want the code count too", res.Counts, err)
		}

		// A code page lasts DefaultCodeTTL.
		time.Sleep(DefaultTTL)
		if _, err := s.Code(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		time.Sleep(DefaultCodeTTL - DefaultTTL)
		if _, err := s.Code(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		api.wantCalls(t, `code "NewStyle" 20 `, `code "NewStyle" 20 next`, `search "NewStyle" 20 [repos=] count [issues pulls]`, `code "NewStyle" 20 `)
	})
}

// A code search that the client refuses for its rate limit fails with
// when code search resumes, and isn't cached, so that it runs once the
// limit lifts.
func TestCodeRateLimited(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(42 * time.Second)
		limited := true
		api := &fakeAPI{t: t, code: func(string, string, int) (core.SearchPage[core.CodeHit], error) {
			if limited {
				return core.SearchPage[core.CodeHit]{}, fmt.Errorf("search code: %w", &core.RateLimitError{Reset: reset})
			}
			return codePage(1, "", styleGo), nil
		}}
		s := New(api)
		_, err := s.Code(t.Context(), CodeQuery{Text: "a"})
		rl, ok := errors.AsType[*core.RateLimitError](err)
		if !ok || !errors.Is(err, core.ErrRateLimited) || !rl.Reset.Equal(reset) {
			t.Fatalf("error = %v, want a rate limit until %v", err, reset)
		}
		if _, ok := s.CachedCode(CodeQuery{Text: "a"}); ok {
			t.Error("a refused search was cached")
		}

		limited = false
		time.Sleep(42 * time.Second)
		if got, err := s.Code(t.Context(), CodeQuery{Text: "a"}); err != nil || len(got.Items) != 1 {
			t.Errorf("Code after the reset = %+v, %v", got, err)
		}
		api.wantCalls(t, `code "a" 20 `, `code "a" 20 `)
	})
}

func TestCodeInvalidQuery(t *testing.T) {
	bad := fmt.Errorf("search code: %w", &core.InvalidQueryError{Reason: "q missing"})
	api := &fakeAPI{t: t, code: func(string, string, int) (core.SearchPage[core.CodeHit], error) {
		return core.SearchPage[core.CodeHit]{}, bad
	}}
	s := New(api)
	if _, err := s.Code(t.Context(), CodeQuery{Text: "repo:"}); !errors.Is(err, core.ErrInvalidQuery) {
		t.Errorf("error = %v, want an invalid query", err)
	}
}
