// Package search finds repositories, issues, pull requests and code on
// GitHub and keeps the results briefly, so typing a query again, or going
// back to it, costs no request.
//
// A first page of repositories, issues or pull requests comes with the
// count of the other two kinds, in one GraphQL query of a single point, so
// a search page can show each kind's count as the user types. GitHub runs
// the searches of one query one after another, so the first pages of the
// other kinds are separate queries, which Prefetch sends side by side. Code search
// has a REST limit of 10 requests a minute, so it runs only when asked for.
// Results may include private data, so they stay in memory.
package search

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// maxPageSize is the largest page GitHub returns.
const maxPageSize = 100

// kinds are the kinds Search looks for, in the order a search page lists
// them.
var kinds = []core.SearchKind{core.SearchRepos, core.SearchIssues, core.SearchPulls}

// API is the part of the GitHub client the service uses.
type API interface {
	Search(ctx context.Context, q github.SearchQuery) (map[core.SearchKind]core.SearchPage[core.SearchHit], error)
	SearchCode(ctx context.Context, query, cursor string, perPage int) (core.SearchPage[core.CodeHit], error)
}

// Query selects a page of search results.
type Query struct {
	// Text is the query in GitHub's search syntax, qualifiers included.
	Text string
	// Kind is core.SearchRepos, core.SearchIssues or core.SearchPulls. The
	// zero value, core.SearchAll, lists the first page of each, one after
	// another, and has no next page.
	Kind core.SearchKind
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many results a page holds. Zero means the service's
	// page size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
}

// normalize returns q with its defaults set: size is the service's page
// size.
func (q Query) normalize(size int) Query {
	q.Text = normalizeText(q.Text)
	q.PageSize = pageSize(q.PageSize, size)
	return q
}

func normalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// pageSize returns n as a page size GitHub accepts: def if n is not
// positive, and at most maxPageSize.
func pageSize(n, def int) int {
	if n <= 0 {
		n = def
	}
	return min(n, maxPageSize)
}

// Result is a page of results of one kind, with the number of results of
// every kind that is known.
type Result struct {
	core.SearchPage[core.SearchHit]
	// Counts holds the total of each kind by kind. A search counts
	// repositories, issues and pull requests at once, and a code search of
	// the same text adds core.SearchCode. A kind whose count isn't known is
	// missing.
	Counts map[core.SearchKind]int
}

// Service searches GitHub through a cache. It is safe for concurrent use.
type Service struct {
	api    API
	pages  *cache.Cache[core.SearchPage[core.SearchHit]]
	code   *cache.Cache[core.SearchPage[core.CodeHit]]
	counts *cache.Cache[map[core.SearchKind]int]
	// pageSize is the size of a page whose query sets none.
	pageSize int

	// mu guards the read, change and write of an entry of counts.
	mu sync.Mutex
}

// New returns a Service that searches with api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default()
	ttl := cache.WithTTL(cmp.Or(o.ttl, d.Cache.TTL.Search))
	capacity := cache.WithCapacity(cmp.Or(o.capacity, d.Cache.Memory.Entries))
	return &Service{
		api:      api,
		pages:    cache.New[core.SearchPage[core.SearchHit]](ttl, capacity),
		code:     cache.New[core.SearchPage[core.CodeHit]](cache.WithTTL(cmp.Or(o.codeTTL, d.Cache.TTL.CodeSearch)), capacity),
		counts:   cache.New[map[core.SearchKind]int](ttl, capacity),
		pageSize: cmp.Or(o.pageSize, d.PageSize.Search),
	}
}

// CachedSearch returns the cached page for q, fresh or stale, without I/O.
// A query without text has no results, which are always known.
func (s *Service) CachedSearch(q Query) (Result, bool) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return Result{}, true
	}
	if q.Kind == core.SearchAll {
		return s.cachedAll(q)
	}
	e, state := s.pages.Get(pageKey(q))
	if state == cache.Miss {
		return Result{}, false
	}
	return Result{SearchPage: e.Value, Counts: s.cachedCounts(q.Text)}, true
}

// Search returns the page for q. A fresh cached page is returned without a
// request, and a query without text returns an empty page without one.
// The first page of any kind comes with the count of the other kinds; a
// later page asks for its kind alone.
func (s *Service) Search(ctx context.Context, q Query) (Result, error) {
	return s.search(ctx, q, true)
}

// Prefetch reads the first page of q.Kind for q.Text into the cache, if it
// isn't there, without counting the other kinds, so that switching to the
// kind is instant. Send one for each kind not on view, alongside the Search
// of the kind on view.
func (s *Service) Prefetch(ctx context.Context, q Query) error {
	q.Cursor = ""
	if q.Kind == core.SearchAll {
		return fmt.Errorf("prefetch %q: name one kind", q.Text)
	}
	_, err := s.search(ctx, q, false)
	return err
}

func (s *Service) search(ctx context.Context, q Query, count bool) (Result, error) {
	q = q.normalize(s.pageSize)
	if q.Text == "" {
		return Result{}, nil
	}
	if q.Kind == core.SearchAll {
		return s.all(ctx, q)
	}
	if !isKind(q.Kind) {
		return Result{}, fmt.Errorf("search %q: cannot search for %q", q.Text, q.Kind)
	}
	e, err := s.pages.Fetch(ctx, pageKey(q), func(ctx context.Context, _ cache.Entry[core.SearchPage[core.SearchHit]], _ bool) (cache.Entry[core.SearchPage[core.SearchHit]], error) {
		// GraphQL responses carry no validators, so a stale page is fetched
		// again in full.
		p, err := s.fetch(ctx, q, count)
		if err != nil {
			return cache.Entry[core.SearchPage[core.SearchHit]]{}, err
		}
		return cache.Entry[core.SearchPage[core.SearchHit]]{Value: p, Tags: []string{allTag}}, nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("search %q: %w", q.Text, err)
	}
	return Result{SearchPage: e.Value, Counts: s.cachedCounts(q.Text)}, nil
}

// fetch asks GitHub for the page q selects, with the count of the other
// kinds if count is set and it is a first page, and keeps the counts.
func (s *Service) fetch(ctx context.Context, q Query, count bool) (core.SearchPage[core.SearchHit], error) {
	sq := github.SearchQuery{Text: q.Text, First: q.PageSize, After: map[core.SearchKind]string{q.Kind: q.Cursor}}
	if count && q.Cursor == "" {
		for _, k := range kinds {
			if k != q.Kind {
				sq.Count = append(sq.Count, k)
			}
		}
	}
	pages, err := s.api.Search(ctx, sq)
	if err != nil {
		return core.SearchPage[core.SearchHit]{}, err
	}
	p, ok := pages[q.Kind]
	if !ok {
		return core.SearchPage[core.SearchHit]{}, fmt.Errorf("github sent no %s", q.Kind)
	}
	totals := make(map[core.SearchKind]int, len(pages))
	for kind, page := range pages {
		totals[kind] = page.Total
	}
	s.addCounts(q.Text, totals)
	return p, nil
}

// all lists the first page of every kind, which one query brings.
func (s *Service) all(ctx context.Context, q Query) (Result, error) {
	if q.Cursor != "" {
		return Result{}, fmt.Errorf("search %q: a search of every kind has one page", q.Text)
	}
	if r, ok := s.cachedAll(q); ok && s.fresh(q) {
		return r, nil
	}
	pages, err := s.api.Search(ctx, github.SearchQuery{Text: q.Text, First: q.PageSize})
	if err != nil {
		return Result{}, fmt.Errorf("search %q: %w", q.Text, err)
	}
	totals := make(map[core.SearchKind]int, len(pages))
	for kind, page := range pages {
		totals[kind] = page.Total
		k := q
		k.Kind = kind
		s.pages.Set(pageKey(k), cache.Entry[core.SearchPage[core.SearchHit]]{Value: page, Tags: []string{allTag}})
	}
	s.addCounts(q.Text, totals)
	r, _ := s.cachedAll(q)
	return r, nil
}

// fresh reports whether the first page of every kind of q is cached and
// fresh.
func (s *Service) fresh(q Query) bool {
	for _, kind := range kinds {
		k := q
		k.Kind = kind
		if _, state := s.pages.Get(pageKey(k)); state != cache.Fresh {
			return false
		}
	}
	return true
}

func (s *Service) cachedAll(q Query) (Result, bool) {
	var res Result
	for _, kind := range kinds {
		k := q
		k.Kind = kind
		e, state := s.pages.Get(pageKey(k))
		if state == cache.Miss {
			return Result{}, false
		}
		res.Items = append(res.Items, e.Value.Items...)
		res.Total += e.Value.Total
	}
	res.Counts = s.cachedCounts(q.Text)
	return res, true
}

// addCounts merges totals into the counts kept for text.
func (s *Service) addCounts(text string, totals map[core.SearchKind]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := countsKey(text)
	e, _ := s.counts.Get(key)
	merged := maps.Clone(e.Value)
	if merged == nil {
		merged = make(map[core.SearchKind]int, len(totals))
	}
	maps.Copy(merged, totals)
	s.counts.Set(key, cache.Entry[map[core.SearchKind]int]{Value: merged, Tags: []string{allTag}})
}

// cachedCounts returns a copy of the counts kept for text, as the cached map
// is shared.
func (s *Service) cachedCounts(text string) map[core.SearchKind]int {
	e, _ := s.counts.Get(countsKey(text))
	return maps.Clone(e.Value)
}

// Invalidate marks every cached page stale, so the next search of each goes
// to GitHub.
func (s *Service) Invalidate() {
	s.pages.InvalidateTag(allTag)
	s.code.InvalidateTag(allTag)
	s.counts.InvalidateTag(allTag)
}

func isKind(k core.SearchKind) bool {
	switch k {
	case core.SearchRepos, core.SearchIssues, core.SearchPulls:
		return true
	default:
		return false
	}
}

// allTag marks every entry, so Invalidate finds them all.
const allTag = "all"

// GitHub matches search terms and qualifiers without regard to case, so
// keys do too.
func pageKey(q Query) string {
	return "search?kind=" + string(q.Kind) + "&page_size=" + strconv.Itoa(q.PageSize) +
		"&cursor=" + url.QueryEscape(q.Cursor) + "&q=" + url.QueryEscape(strings.ToLower(q.Text))
}

func countsKey(text string) string {
	return "search/counts?q=" + url.QueryEscape(strings.ToLower(text))
}
