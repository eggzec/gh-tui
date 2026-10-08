package refs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

var (
	here  = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	other = core.RepoRef{Owner: "cli", Name: "go-gh"}
	query = Query{Repo: here, Number: 231, Pull: true}
)

// fakeAPI answers with its func fields and records the calls. A nil field
// answers with nothing.
type fakeAPI struct {
	sources  func() (core.RefSources, error)
	resolve  func([]core.Target) ([]core.Reference, error)
	mentions func(before string, last int) (core.Page[core.Reference], error)

	mu    sync.Mutex
	calls []string
	// resolved holds the targets of each call of ResolveReferences.
	resolved [][]core.Target
}

func (f *fakeAPI) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeAPI) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeAPI) wantCalls(t *testing.T, want ...string) {
	t.Helper()
	if got := f.Calls(); !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func (f *fakeAPI) References(_ context.Context, repo core.RepoRef, number int, pull bool) (core.RefSources, error) {
	f.record(fmt.Sprintf("references %s#%d %v", repo, number, pull))
	if f.sources == nil {
		return core.RefSources{}, nil
	}
	return f.sources()
}

func (f *fakeAPI) ResolveReferences(_ context.Context, targets []core.Target) ([]core.Reference, error) {
	f.record(fmt.Sprintf("resolve %d", len(targets)))
	f.mu.Lock()
	f.resolved = append(f.resolved, slices.Clone(targets))
	f.mu.Unlock()
	if f.resolve != nil {
		return f.resolve(targets)
	}
	return readAll(targets), nil
}

func (f *fakeAPI) Mentions(_ context.Context, repo core.RepoRef, number int, pull bool, before string, last int) (core.Page[core.Reference], error) {
	f.record(fmt.Sprintf("mentions %s#%d %v before=%q last=%d", repo, number, pull, before, last))
	if f.mentions == nil {
		return core.Page[core.Reference]{}, nil
	}
	return f.mentions(before, last)
}

// readAll answers for every target as an open issue titled by its key.
func readAll(targets []core.Target) []core.Reference {
	out := make([]core.Reference, len(targets))
	for i, t := range targets {
		t.Kind = core.KindIssue
		out[i] = core.Reference{Target: t, Title: core.RefKey(t), State: core.StateOpen}
	}
	return out
}

func item(repo core.RepoRef, n int, state core.State) core.Reference {
	return core.Reference{Target: core.Target{Repo: repo, Number: n, Kind: core.KindIssue}, Title: core.RefKey(core.Target{Repo: repo, Number: n}), State: state}
}

func written(where, by string) core.RefOrigin {
	return core.RefOrigin{Group: core.RefWritten, Where: where, By: by}
}

func closes() core.RefOrigin { return core.RefOrigin{Group: core.RefClosing, Where: "closes"} }

func keys(refs []core.Reference) []string {
	out := make([]string, 0, len(refs))
	for i := range refs {
		out = append(out, core.RefKey(refs[i].Target))
	}
	return out
}

// storeFor returns a store that a second service of the test can share.
func storeFor(t *testing.T) cache.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// mergeSources is a pull request #231 that links in every way.
func mergeSources() core.RefSources {
	return core.RefSources{
		Texts: []core.RefText{
			{Origin: written("body", ""), Text: "Fixes #198 and #12, see octo/secret#9. It is #231, https://github.com/EggZec/GH-TUI/pull/231 too."},
			{Origin: written("comment", "bob"), Text: "Also #12 and cli/go-gh#412 `#555`"},
			{Origin: core.RefOrigin{Group: core.RefWritten, Where: "review", By: "alice"}, Text: "#12 again, and #41"},
		},
		Closing: []core.Reference{item(here, 198, core.StateOpen)},
		Linked: []core.RefLink{
			{Ref: item(here, 40, core.StateOpen)},
			{Ref: item(here, 40, core.StateOpen), Disconnected: true},
			{Ref: item(here, 41, core.StateClosed)},
			{Ref: item(here, 41, core.StateClosed)},
			{Ref: item(here, 198, core.StateOpen)},
		},
		Mentioned: 47, CommentsRead: 2, CommentsTotal: 340, ReviewsRead: 1, ReviewsTotal: 1,
	}
}

func TestReferencesMerge(t *testing.T) {
	api := &fakeAPI{sources: func() (core.RefSources, error) { return mergeSources(), nil }}
	api.resolve = func(targets []core.Target) ([]core.Reference, error) {
		out := readAll(targets)
		for i := range out {
			if targets[i].Repo.Owner == "octo" {
				out[i] = core.Reference{Target: targets[i], Problem: "not found, or private"}
			}
		}
		return out, nil
	}
	s := New(api)

	got, err := s.References(t.Context(), query)
	if err != nil {
		t.Fatalf("References: %v", err)
	}

	// Closing: the open first, and #40, which was connected and then
	// disconnected, is gone. #41 is closed, which is the written one too.
	if want := []string{"eggzec/gh-tui#198", "eggzec/gh-tui#41"}; !slices.Equal(keys(got.Closing), want) {
		t.Errorf("Closing = %q, want %q", keys(got.Closing), want)
	}
	// One entry whatever the number of places, its origins strongest first,
	// and the same place not twice.
	wantOrigins := []core.RefOrigin{closes(), written("body", "")}
	if o := got.Closing[0].Origins; !slices.Equal(o, wantOrigins) {
		t.Errorf("origins of #198 = %+v, want %+v", o, wantOrigins)
	}
	wantOrigins = []core.RefOrigin{closes(), written("review", "alice")}
	if o := got.Closing[1].Origins; !slices.Equal(o, wantOrigins) {
		t.Errorf("origins of #41 = %+v, want %+v", o, wantOrigins)
	}
	// Written: by first appearance. The item itself and code are no links.
	if want := []string{"eggzec/gh-tui#12", "octo/secret#9", "cli/go-gh#412"}; !slices.Equal(keys(got.Written), want) {
		t.Errorf("Written = %q, want %q", keys(got.Written), want)
	}
	wantOrigins = []core.RefOrigin{written("body", ""), written("comment", "bob"), written("review", "alice")}
	if o := got.Written[0].Origins; !slices.Equal(o, wantOrigins) {
		t.Errorf("origins of #12 = %+v, want %+v", o, wantOrigins)
	}
	if r := got.Written[1]; r.Problem != "not found, or private" || r.Target.Repo.Owner != "octo" {
		t.Errorf("octo/secret#9 = %+v, want it unreadable", r)
	}
	// Only what the texts name and GitHub's links lack is read.
	if len(api.resolved) != 1 || len(api.resolved[0]) != 3 || api.resolved[0][0] != (core.Target{Repo: here, Number: 12}) {
		t.Errorf("resolved = %v, want #12, octo/secret#9 and cli/go-gh#412 in one call", api.resolved)
	}
	if got.Mentioned != 47 || got.CommentsRead != 2 || got.CommentsTotal != 340 || got.ReviewsRead != 1 || got.ReviewsTotal != 1 || got.Unresolved != 0 {
		t.Errorf("counts = %+v, want those of the sources", got)
	}
	if got.ReadAt.IsZero() || got.Stale || got.Offline || got.Limited {
		t.Errorf("flags = %+v, want a fresh read", got)
	}
	// Nothing about the mentions is read until asked.
	for _, c := range api.Calls() {
		if strings.HasPrefix(c, "mentions") {
			t.Errorf("call %q, want no read of the mentions", c)
		}
	}
}

func TestReferencesOfAnIssue(t *testing.T) {
	api := &fakeAPI{sources: func() (core.RefSources, error) {
		pr := item(here, 300, core.StateMerged)
		pr.Target.Kind = core.KindPull
		return core.RefSources{
			Closing: []core.Reference{pr},
			Closers: []core.Reference{pr},
			Linked:  []core.RefLink{{Ref: item(here, 301, core.StateOpen)}},
		}, nil
	}}
	s := New(api)

	got, err := s.References(t.Context(), Query{Repo: here, Number: 231})
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if want := []string{"eggzec/gh-tui#301", "eggzec/gh-tui#300"}; !slices.Equal(keys(got.Closing), want) {
		t.Fatalf("Closing = %q, want %q", keys(got.Closing), want)
	}
	if o := got.Closing[1].Origins; len(o) != 1 || o[0] != (core.RefOrigin{Group: core.RefClosing, Where: "closed by"}) {
		t.Errorf("origins of #300 = %+v, want closed by, once", o)
	}
	api.wantCalls(t, "references eggzec/gh-tui#231 false")
}

func TestReferencesHost(t *testing.T) {
	text := "https://ghe.example.com/o/r/pull/5 https://ghe.example.com/api/v3/repos/o/r/issues/6 https://github.com/o/r/pull/7 #8"
	src := func() (core.RefSources, error) {
		return core.RefSources{Texts: []core.RefText{{Origin: written("body", ""), Text: text}}}, nil
	}
	api := &fakeAPI{sources: src}
	s := New(api, WithHost("ghe.example.com"))

	got, err := s.References(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"o/r#5", "o/r#6", "eggzec/gh-tui#8"}; !slices.Equal(keys(got.Written), want) {
		t.Errorf("Written = %q, want %q: the links of the session's host, and none of github.com", keys(got.Written), want)
	}
}

func TestResolveBatches(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 130; i++ {
		fmt.Fprintf(&b, "#%d ", i)
	}
	api := &fakeAPI{sources: func() (core.RefSources, error) {
		return core.RefSources{Texts: []core.RefText{{Origin: written("body", ""), Text: b.String()}}}, nil
	}}
	s := New(api)

	got, err := s.References(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	// 231 isn't among them, so no one is left out as the item itself.
	sizes, numbers := make([]int, 0, len(api.resolved)), []int{}
	for _, call := range api.resolved {
		sizes = append(sizes, len(call))
		for _, tg := range call {
			numbers = append(numbers, tg.Number)
		}
	}
	slices.Sort(numbers)
	if want := []int{20, 20, 20, 20, 20}; !slices.Equal(sizes, want) || numbers[0] != 1 || numbers[len(numbers)-1] != maxResolve {
		t.Fatalf("resolve calls of %v targets from #%d to #%d, want 5 of 20, for the first %d", sizes, numbers[0], numbers[len(numbers)-1], maxResolve)
	}
	if len(got.Written) != maxResolve || got.Unresolved != 30 {
		t.Errorf("%d written and %d unresolved, want %d and 30", len(got.Written), got.Unresolved, maxResolve)
	}
	// Reading the batches at once keeps the order of the texts.
	for i := range got.Written {
		if got.Written[i].Target.Number != i+1 {
			t.Fatalf("Written[%d] is #%d, want the order of the text", i, got.Written[i].Target.Number)
		}
	}
}

// A batch that is refused after others were read leaves what they read.
func TestResolveKeepsEarlierBatches(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 45; i++ {
		fmt.Fprintf(&b, "#%d ", i)
	}
	api := &fakeAPI{
		sources: func() (core.RefSources, error) {
			return core.RefSources{Texts: []core.RefText{{Origin: written("body", ""), Text: b.String()}}}, nil
		},
		resolve: func(targets []core.Target) ([]core.Reference, error) {
			if targets[0].Number == 21 {
				return nil, &core.RateLimitError{Reset: time.Now().Add(time.Hour)}
			}
			return readAll(targets), nil
		},
	}
	s := New(api)
	got, err := s.References(t.Context(), query)
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(got.Written) != 25 || got.Failed != 20 || got.FailedWhy != "rate limited" || got.Unresolved != 0 || !got.Limited {
		t.Errorf("%d written, %d failed because %q, %d unresolved, limited %v; want 25, 20, the rate limit, 0 and limited",
			len(got.Written), got.Failed, got.FailedWhy, got.Unresolved, got.Limited)
	}
	// Not kept as fresh: the next read asks again, and now it is read.
	api.resolve = func(targets []core.Target) ([]core.Reference, error) { return readAll(targets), nil }
	again, err := s.References(t.Context(), query)
	if err != nil || len(again.Written) != 45 || again.Failed != 0 || again.Limited {
		t.Errorf("second read = %d written, %d failed, %v; want all 45 read", len(again.Written), again.Failed, err)
	}
	if n := len(api.resolved); n != 3+3 {
		t.Errorf("%d resolve calls, want 3 and 3: the failed read was served from the cache", n)
	}
	if _, err := s.References(t.Context(), query); err != nil || len(api.resolved) != 6 {
		t.Errorf("a complete read was not kept: %d calls", len(api.resolved))
	}
	for _, r := range got.Written {
		if n := r.Target.Number; n > 20 && n < 41 {
			t.Errorf("#%d is written, though its batch failed", n)
		}
	}
}

func TestResolveFails(t *testing.T) {
	reset := time.Now().Add(time.Hour)
	api := &fakeAPI{
		sources: func() (core.RefSources, error) {
			return core.RefSources{Texts: []core.RefText{{Origin: written("body", ""), Text: "#1"}}}, nil
		},
		resolve: func([]core.Target) ([]core.Reference, error) {
			return nil, fmt.Errorf("resolve references: %w", &core.RateLimitError{Reset: reset})
		},
	}
	s := New(api)

	_, err := s.References(t.Context(), query)
	if rl, ok := errors.AsType[*core.RateLimitError](err); !ok || !rl.Reset.Equal(reset) {
		t.Errorf("error = %v, want the rate limit", err)
	}
	if _, ok := s.CachedReferences(query); ok {
		t.Error("a failed read was cached")
	}
	api.resolve = func([]core.Target) ([]core.Reference, error) { return nil, nil }
	if _, err := s.References(t.Context(), query); err == nil {
		t.Error("a client that answers for none of the items was believed")
	}
}

func TestReferencesCached(t *testing.T) {
	text := "#12"
	api := &fakeAPI{sources: func() (core.RefSources, error) {
		return core.RefSources{Texts: []core.RefText{{Origin: written("comment", "bob"), Text: text}}}, nil
	}}
	s := New(api)
	if _, ok := s.CachedReferences(query); ok {
		t.Fatal("CachedReferences found something before any read")
	}

	first, err := s.References(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.References(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	api.wantCalls(t, "references eggzec/gh-tui#231 true", "resolve 1")
	cached, ok := s.CachedReferences(query)
	if !ok || !slices.Equal(keys(cached.Written), keys(first.Written)) || !slices.Equal(keys(second.Written), keys(first.Written)) {
		t.Errorf("cached = %+v, %v; want what was read", cached, ok)
	}
	// Another item, and an issue of the same number, are others.
	if _, ok := s.CachedReferences(Query{Repo: here, Number: 232, Pull: true}); ok {
		t.Error("an item was found under another's number")
	}
	if _, ok := s.CachedReferences(Query{Repo: here, Number: 231}); ok {
		t.Error("an issue was found under the pull request's key")
	}
	// GitHub ignores the case of a repository.
	if _, ok := s.CachedReferences(Query{Repo: core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}, Number: 231, Pull: true}); !ok {
		t.Error("the cache told apart the cases of a repository")
	}

	// An edited comment: after the item is invalidated, the link is gone.
	text = "no links now"
	s.Invalidate(here, 231)
	stale, ok := s.CachedReferences(query)
	if !ok || len(stale.Written) != 1 {
		t.Errorf("an invalidated read = %+v, %v; want it served until read again", stale, ok)
	}
	again, err := s.References(t.Context(), query)
	if err != nil || len(again.Written) != 0 {
		t.Errorf("References after the edit = %+v, %v; want #12 dropped", again, err)
	}
	if len(api.Calls()) != 3 {
		t.Errorf("calls = %q, want one more read", api.Calls())
	}
}

func TestReferencesKept(t *testing.T) {
	store := storeFor(t)
	sources := func() (core.RefSources, error) {
		return core.RefSources{Texts: []core.RefText{{Origin: written("body", ""), Text: "#12"}}, Mentioned: 3}, nil
	}
	first := New(&fakeAPI{sources: sources}, WithStore(store))
	if _, err := first.References(t.Context(), query); err != nil {
		t.Fatal(err)
	}
	// A TTL that has passed by the next session.
	next := func(api *fakeAPI) *Service { return New(api, WithStore(store), WithTTL(time.Nanosecond)) }

	t.Run("stale", func(t *testing.T) {
		api := &fakeAPI{sources: sources}
		s := next(api)
		got, err := s.References(t.Context(), query)
		if err != nil || !got.Stale || len(got.Written) != 1 || got.Mentioned != 3 {
			t.Fatalf("References = %+v, %v; want the kept read, stale", got, err)
		}
		api.wantCalls(t)
		// Served again, until a read asks again.
		if again, _ := s.References(t.Context(), query); !again.Stale {
			t.Error("the kept read was served fresh")
		}
		q := query
		q.Again = true
		got, err = s.References(t.Context(), q)
		if err != nil || got.Stale {
			t.Errorf("References with Again = %+v, %v; want a read from GitHub", got, err)
		}
		api.wantCalls(t, "references eggzec/gh-tui#231 true", "resolve 1")
	})
	t.Run("offline", func(t *testing.T) {
		down := fmt.Errorf("%w: connection refused", core.ErrOffline)
		s := next(&fakeAPI{sources: func() (core.RefSources, error) { return core.RefSources{}, down }})
		q := query
		q.Again = true
		got, err := s.References(t.Context(), q)
		if err != nil || !got.Offline || got.Limited || len(got.Written) != 1 {
			t.Errorf("References = %+v, %v; want the kept read, offline", got, err)
		}
	})
	t.Run("limited", func(t *testing.T) {
		s := next(&fakeAPI{sources: func() (core.RefSources, error) {
			return core.RefSources{}, &core.RateLimitError{Reset: time.Now().Add(time.Hour)}
		}})
		q := query
		q.Again = true
		got, err := s.References(t.Context(), q)
		if err != nil || !got.Limited || got.Offline || len(got.Written) != 1 {
			t.Errorf("References = %+v, %v; want the kept read, limited", got, err)
		}
	})
	t.Run("offline with nothing kept", func(t *testing.T) {
		down := fmt.Errorf("%w: connection refused", core.ErrOffline)
		s := New(&fakeAPI{sources: func() (core.RefSources, error) { return core.RefSources{}, down }}, WithStore(storeFor(t)))
		if _, err := s.References(t.Context(), query); !errors.Is(err, core.ErrOffline) {
			t.Errorf("error = %v, want the offline error", err)
		}
	})
	t.Run("refused", func(t *testing.T) {
		s := next(&fakeAPI{sources: func() (core.RefSources, error) { return core.RefSources{}, core.ErrNotFound }})
		q := query
		q.Again = true
		if _, err := s.References(t.Context(), q); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("error = %v, want not found", err)
		}
		// What the account may no longer see is dropped.
		if got, err := next(&fakeAPI{sources: sources}).References(t.Context(), query); err != nil || got.Stale {
			t.Errorf("References = %+v, %v; want nothing kept to serve", got, err)
		}
	})
}

func mentionPage(next string, items ...core.Reference) core.Page[core.Reference] {
	for i := range items {
		items[i].Origins = []core.RefOrigin{{Group: core.RefMentioned, Where: "mentioned", By: "dave"}}
	}
	return core.Page[core.Reference]{Items: items, Next: next}
}

func TestMentionsPage(t *testing.T) {
	api := &fakeAPI{mentions: func(before string, _ int) (core.Page[core.Reference], error) {
		if before == "" {
			return mentionPage("c2", item(here, 240, core.StateOpen), item(other, 7, core.StateClosed)), nil
		}
		return mentionPage("", item(here, 100, core.StateOpen)), nil
	}}
	s := New(api, WithPageSize(40))
	q := MentionsQuery{Repo: here, Number: 231, Pull: true}
	if _, ok := s.CachedMentions(q); ok {
		t.Fatal("CachedMentions found a page before any read")
	}

	p1, err := s.Mentions(t.Context(), q)
	if err != nil || p1.Next != "c2" || len(p1.Items) != 2 {
		t.Fatalf("Mentions = %+v, %v; want the first page", p1, err)
	}
	q.Cursor = p1.Next
	p2, err := s.Mentions(t.Context(), q)
	if err != nil || p2.Next != "" || len(p2.Items) != 1 {
		t.Fatalf("Mentions after the cursor = %+v, %v; want the last page", p2, err)
	}
	// The cursor and the size are in the request, and each page is cached
	// on its own.
	api.wantCalls(t,
		`mentions eggzec/gh-tui#231 true before="" last=40`,
		`mentions eggzec/gh-tui#231 true before="c2" last=40`)
	if _, err := s.Mentions(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	if c, ok := s.CachedMentions(MentionsQuery{Repo: here, Number: 231, Pull: true}); !ok || len(c.Items) != 2 {
		t.Errorf("CachedMentions = %+v, %v; want the first page", c, ok)
	}
	if len(api.Calls()) != 2 {
		t.Errorf("calls = %q, want no more reads of cached pages", api.Calls())
	}
	// A size of its own is another page, clamped to GitHub's largest.
	for _, size := range []int{25, 5000} {
		if _, err := s.Mentions(t.Context(), MentionsQuery{Repo: here, Number: 231, Pull: true, PageSize: size}); err != nil {
			t.Fatal(err)
		}
	}
	calls := api.Calls()
	if got := calls[len(calls)-2:]; got[0] != `mentions eggzec/gh-tui#231 true before="" last=25` || got[1] != `mentions eggzec/gh-tui#231 true before="" last=100` {
		t.Errorf("last calls = %q, want sizes 25 and 100", got)
	}
}

func TestMentionsDefaultSize(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	if _, err := s.Mentions(t.Context(), MentionsQuery{Repo: here, Number: 1}); err != nil {
		t.Fatal(err)
	}
	api.wantCalls(t, `mentions eggzec/gh-tui#1 false before="" last=100`)
}

// A page leaves out what References lists in its groups, whichever is
// read first, and an item whose mention closes this one moves to Closing.
func TestMentionsLeaveOutAndPromote(t *testing.T) {
	api := &fakeAPI{
		sources: func() (core.RefSources, error) {
			return core.RefSources{
				Texts:     []core.RefText{{Origin: written("body", ""), Text: "#12"}},
				Closing:   []core.Reference{item(here, 198, core.StateOpen)},
				Mentioned: 5,
			}, nil
		},
		mentions: func(string, int) (core.Page[core.Reference], error) {
			closer := item(here, 239, core.StateMerged)
			closer.Origins = []core.RefOrigin{closes(), {Group: core.RefMentioned, Where: "mentioned", By: "bob"}}
			p := mentionPage("", item(here, 240, core.StateOpen), item(here, 198, core.StateOpen), item(here, 12, core.StateClosed), item(other, 1, core.StateOpen))
			p.Items = append(p.Items, closer)
			return p, nil
		},
	}
	s := New(api)
	q := MentionsQuery{Repo: here, Number: 231, Pull: true}

	// Read before References: nothing to leave out yet.
	if p, err := s.Mentions(t.Context(), q); err != nil || len(p.Items) != 5 {
		t.Fatalf("Mentions = %+v, %v; want all 5", p, err)
	}
	refs, err := s.References(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"eggzec/gh-tui#198"}; !slices.Equal(keys(refs.Closing), want) {
		t.Fatalf("Closing = %q, want %q", keys(refs.Closing), want)
	}
	p, err := s.Mentions(t.Context(), q)
	if err != nil {
		t.Fatal(err)
	}
	// #198 and #12 are in the groups above; #239 closes the item. What is
	// left is the mentions alone.
	if want := []string{"eggzec/gh-tui#240", "cli/go-gh#1"}; !slices.Equal(keys(p.Items), want) {
		t.Errorf("Mentions = %q, want %q", keys(p.Items), want)
	}
	cp, ok := s.CachedMentions(q)
	if !ok || !slices.Equal(keys(cp.Items), keys(p.Items)) {
		t.Errorf("CachedMentions = %q, %v; want the same page", keys(cp.Items), ok)
	}
	refs, _ = s.CachedReferences(query)
	if want := []string{"eggzec/gh-tui#198", "eggzec/gh-tui#239"}; !slices.Equal(keys(refs.Closing), want) {
		t.Fatalf("Closing = %q, want #239 joined, merged after the open one", keys(refs.Closing))
	}
	if o := refs.Closing[1].Origins; len(o) != 2 || o[0].Group != core.RefClosing || o[1].Group != core.RefMentioned || o[1].By != "bob" {
		t.Errorf("origins of #239 = %+v, want closes, then mentioned by bob", o)
	}
	// The count is GitHub's, whatever the rows.
	if refs.Mentioned != 5 {
		t.Errorf("Mentioned = %d, want GitHub's 5", refs.Mentioned)
	}
	if got := len(api.Calls()); got != 3 {
		t.Errorf("calls = %q, want a read of the mentions once and of the references once", api.Calls())
	}
}

func TestMentionsPromoteFromWritten(t *testing.T) {
	api := &fakeAPI{
		sources: func() (core.RefSources, error) {
			return core.RefSources{Texts: []core.RefText{{Origin: written("body", ""), Text: "#239"}}}, nil
		},
		mentions: func(string, int) (core.Page[core.Reference], error) {
			closer := item(here, 239, core.StateMerged)
			closer.Origins = []core.RefOrigin{closes(), {Group: core.RefMentioned, Where: "mentioned"}}
			return core.Page[core.Reference]{Items: []core.Reference{closer}}, nil
		},
	}
	s := New(api)
	if _, err := s.References(t.Context(), query); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Mentions(t.Context(), MentionsQuery{Repo: here, Number: 231, Pull: true}); err != nil {
		t.Fatal(err)
	}
	refs, _ := s.CachedReferences(query)
	if len(refs.Written) != 0 || len(refs.Closing) != 1 {
		t.Fatalf("References = %+v, want #239 moved to Closing", refs)
	}
	want := []core.RefOrigin{closes(), written("body", ""), {Group: core.RefMentioned, Where: "mentioned"}}
	if o := refs.Closing[0].Origins; !slices.Equal(o, want) {
		t.Errorf("origins = %+v, want %+v", o, want)
	}
}

func TestMentionsInvalidate(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	q := MentionsQuery{Repo: here, Number: 231, Pull: true}
	if _, err := s.Mentions(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	s.Invalidate(here, 231)
	if _, ok := s.CachedMentions(q); !ok {
		t.Error("an invalidated page is no longer served")
	}
	if _, err := s.Mentions(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	if len(api.Calls()) != 2 {
		t.Errorf("calls = %q, want the page read again", api.Calls())
	}
	// Another item's pages stay fresh.
	other := MentionsQuery{Repo: here, Number: 232, Pull: true}
	if _, err := s.Mentions(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	s.Invalidate(here, 231)
	if _, err := s.Mentions(t.Context(), other); err != nil || len(api.Calls()) != 3 {
		t.Errorf("calls = %q, want another item's page kept fresh", api.Calls())
	}
}

func TestMentionsKept(t *testing.T) {
	store := storeFor(t)
	q := MentionsQuery{Repo: here, Number: 231, Pull: true}
	page := func(string, int) (core.Page[core.Reference], error) {
		return mentionPage("c", item(here, 240, core.StateOpen)), nil
	}
	if _, err := New(&fakeAPI{mentions: page}, WithStore(store)).Mentions(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	next := func(api *fakeAPI) *Service { return New(api, WithStore(store), WithTTL(time.Nanosecond)) }

	api := &fakeAPI{mentions: page}
	got, err := next(api).Mentions(t.Context(), q)
	if err != nil || !got.Stale || len(got.Items) != 1 || got.Next != "c" {
		t.Fatalf("Mentions = %+v, %v; want the kept page, stale", got, err)
	}
	api.wantCalls(t)

	q.Again = true
	down := fmt.Errorf("%w: connection refused", core.ErrOffline)
	got, err = next(&fakeAPI{mentions: func(string, int) (core.Page[core.Reference], error) {
		return core.Page[core.Reference]{}, down
	}}).Mentions(t.Context(), q)
	if err != nil || !got.Offline || len(got.Items) != 1 {
		t.Errorf("offline: Mentions = %+v, %v; want the kept page, offline", got, err)
	}
	got, err = next(&fakeAPI{mentions: func(string, int) (core.Page[core.Reference], error) {
		return core.Page[core.Reference]{}, &core.RateLimitError{Reset: time.Now().Add(time.Hour)}
	}}).Mentions(t.Context(), q)
	if err != nil || !got.Limited || len(got.Items) != 1 {
		t.Errorf("limited: Mentions = %+v, %v; want the kept page, limited", got, err)
	}
	q.Cursor = "never read"
	if _, err := next(&fakeAPI{mentions: func(string, int) (core.Page[core.Reference], error) {
		return core.Page[core.Reference]{}, down
	}}).Mentions(t.Context(), q); !errors.Is(err, core.ErrOffline) {
		t.Errorf("offline with no kept page: error = %v, want the offline error", err)
	}
}

func TestReferencesUpdated(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	t0 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	q := query
	q.Updated = t0
	for range 2 {
		if _, err := s.References(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	if len(api.Calls()) != 1 {
		t.Fatalf("calls = %q, want one read for an item that didn't change", api.Calls())
	}
	// A comment since: the item is updated, and the read is made again.
	q.Updated = t0.Add(time.Minute)
	got, err := s.References(t.Context(), q)
	if err != nil || len(api.Calls()) != 2 || !got.ItemUpdated.Equal(q.Updated) {
		t.Fatalf("References = %+v, %v after %q; want a new read for the new time", got, err, api.Calls())
	}
	if _, err := s.References(t.Context(), q); err != nil || len(api.Calls()) != 2 {
		t.Errorf("calls = %q, want the new read kept", api.Calls())
	}
	// A caller that doesn't know the time is served what is cached.
	if _, err := s.References(t.Context(), query); err != nil || len(api.Calls()) != 2 {
		t.Errorf("calls = %q, want the cache without a time", api.Calls())
	}
}

func TestReferencesUpdatedKept(t *testing.T) {
	store := storeFor(t)
	t0 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	q := query
	q.Updated = t0
	if _, err := New(&fakeAPI{}, WithStore(store)).References(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{}
	s := New(api, WithStore(store))
	if got, err := s.References(t.Context(), q); err != nil || got.Stale || len(api.Calls()) != 0 {
		t.Fatalf("References = %+v, %v after %q; want the kept read for the same time", got, err, api.Calls())
	}
	q.Updated = t0.Add(time.Hour)
	if _, err := s.References(t.Context(), q); err != nil || len(api.Calls()) != 1 {
		t.Errorf("calls = %q, want the kept read of an older item read again", api.Calls())
	}
}

// What closes the item is one origin however it was found, and two links
// that resolve to one item are one row.
func TestReferencesDedupeOrigins(t *testing.T) {
	pr := item(here, 300, core.StateMerged)
	api := &fakeAPI{
		sources: func() (core.RefSources, error) {
			return core.RefSources{
				Closing: []core.Reference{pr},
				Closers: []core.Reference{pr, pr},
				Texts:   []core.RefText{{Origin: written("body", ""), Text: "https://github.com/EggZec/GH-TUI/issues/12 and #12"}},
			}, nil
		},
	}
	got, err := New(api).References(t.Context(), Query{Repo: here, Number: 231})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Closing) != 1 || len(got.Closing[0].Origins) != 1 {
		t.Errorf("Closing = %+v, want one entry with one origin, \"closed by\", said once", got.Closing)
	}
	if len(got.Written) != 1 || len(got.Written[0].Origins) != 1 {
		t.Errorf("Written = %+v, want #12 once", got.Written)
	}
	if o := refOrigins([]core.RefOrigin{{Group: core.RefClosing, Where: "closed by"}, {Group: core.RefClosing, Where: "closed by", By: "bob"}}); len(o) != 1 || o[0].By != "bob" {
		t.Errorf("refOrigins = %+v, want one origin with its author", o)
	}
}

func TestMentionsDedupe(t *testing.T) {
	more := func(n int) core.Reference {
		r := item(here, n, core.StateOpen)
		r.Origins = []core.RefOrigin{{Group: core.RefMentioned, Where: "mentioned", By: "u" + strconv.Itoa(n)}}
		return r
	}
	again := more(5)
	again.Origins[0].By = "other"
	api := &fakeAPI{mentions: func(before string, _ int) (core.Page[core.Reference], error) {
		if before == "" {
			return core.Page[core.Reference]{Items: []core.Reference{more(5), more(6), again}, Next: "c"}, nil
		}
		return core.Page[core.Reference]{Items: []core.Reference{more(6), more(7)}}, nil
	}}
	s := New(api)
	q := MentionsQuery{Repo: here, Number: 231, Pull: true}
	p1, err := s.Mentions(t.Context(), q)
	if err != nil || !slices.Equal(keys(p1.Items), []string{"eggzec/gh-tui#5", "eggzec/gh-tui#6"}) || len(p1.Items[0].Origins) != 2 {
		t.Fatalf("first page = %+v, %v; want #5 once with both mentions, and #6", p1, err)
	}
	q.Cursor = p1.Next
	p2, err := s.Mentions(t.Context(), q)
	if err != nil || !slices.Equal(keys(p2.Items), []string{"eggzec/gh-tui#7"}) {
		t.Errorf("second page = %+v, %v; want only #7, as #6 is above", p2, err)
	}
	if c, ok := s.CachedMentions(q); !ok || !slices.Equal(keys(c.Items), keys(p2.Items)) {
		t.Errorf("CachedMentions = %+v, %v; want the same page", c, ok)
	}
}

// Batches finish in any order, and the links stay in the order of the
// texts.
func TestResolveOutOfOrder(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&b, "#%d ", i)
	}
	api := &fakeAPI{
		sources: func() (core.RefSources, error) {
			return core.RefSources{Texts: []core.RefText{{Origin: written("body", ""), Text: b.String()}}}, nil
		},
		resolve: func(targets []core.Target) ([]core.Reference, error) {
			if targets[0].Number == 1 {
				time.Sleep(50 * time.Millisecond)
			}
			return readAll(targets), nil
		},
	}
	got, err := New(api).References(t.Context(), query)
	if err != nil || len(got.Written) != 60 {
		t.Fatalf("References = %d written, %v", len(got.Written), err)
	}
	for i := range got.Written {
		if got.Written[i].Target.Number != i+1 {
			t.Fatalf("Written[%d] is #%d, want the order of the text", i, got.Written[i].Target.Number)
		}
	}
}

// Invalidating an item forgets how its pages follow each other, and no
// other item's.
func TestInvalidateClearsChain(t *testing.T) {
	api := &fakeAPI{mentions: func(_ string, _ int) (core.Page[core.Reference], error) {
		return core.Page[core.Reference]{Next: "c"}, nil
	}}
	s := New(api)
	for _, n := range []int{1, 2} {
		if _, err := s.Mentions(t.Context(), MentionsQuery{Repo: here, Number: n}); err != nil {
			t.Fatal(err)
		}
	}
	count := func() (n int) {
		s.chain.Range(func(_, _ any) bool { n++; return true })
		return n
	}
	if count() != 2 {
		t.Fatalf("chain holds %d links, want one for each item", count())
	}
	s.Invalidate(here, 1)
	if count() != 1 {
		t.Errorf("chain holds %d links after Invalidate, want only the other item's", count())
	}
}
