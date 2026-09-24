// Package search finds repositories, issues, pull requests and code on
// GitHub and keeps the results briefly, so typing a query again, or going
// back to it, costs no request.
//
// Repositories, issues and pull requests are searched together, in one
// GraphQL query that costs a single point and counts every kind, so a
// search page can show each kind's count as the user types. Code search
// has a REST limit of 10 requests a minute, so it runs only when asked for.
// Results may include private data, so they stay in memory.
package search

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// Defaults of a Service and a Query.
const (
	// DefaultPageSize is the page size of a Query that sets none.
	DefaultPageSize = 20
	// DefaultTTL is short: results change, and a query typed again soon
	// after is the case the cache is for.
	DefaultTTL = 30 * time.Second
	// DefaultCodeTTL is longer, since code search allows only 10 requests
	// a minute and GitHub indexes code with a delay anyway.
	DefaultCodeTTL = time.Minute
	// DefaultCapacity is how many pages of results a Service keeps.
	DefaultCapacity = 256
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
	// PageSize is how many results a page holds. Zero means
	// DefaultPageSize, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
}

func (q Query) normalize() Query {
	q.Text = normalizeText(q.Text)
	q.PageSize = pageSize(q.PageSize)
	return q
}

func normalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func pageSize(n int) int {
	if n <= 0 {
		return DefaultPageSize
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

	// mu guards the read, change and write of an entry of counts, and
	// codeReset.
	mu sync.Mutex
	// codeReset is when code search may run again after GitHub said it
	// ran out, or the zero time.
	codeReset time.Time
}

// New returns a Service that searches with api.
func New(api API, opts ...Option) *Service {
	o := options{ttl: DefaultTTL, codeTTL: DefaultCodeTTL, capacity: DefaultCapacity}
	for _, opt := range opts {
		opt(&o)
	}
	return &Service{
		api:    api,
		pages:  cache.New[core.SearchPage[core.SearchHit]](cache.WithTTL(o.ttl), cache.WithCapacity(o.capacity)),
		code:   cache.New[core.SearchPage[core.CodeHit]](cache.WithTTL(o.codeTTL), cache.WithCapacity(o.capacity)),
		counts: cache.New[map[core.SearchKind]int](cache.WithTTL(o.ttl), cache.WithCapacity(o.capacity)),
	}
}

// CachedSearch returns the cached page for q, fresh or stale, without I/O.
// A query without text has no results, which are always known.
func (s *Service) CachedSearch(q Query) (Result, bool) {
	q = q.normalize()
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
// The first page of any kind comes with the first page of the other kinds,
// which are cached for when the user switches to them; a later page asks
// for its kind alone.
func (s *Service) Search(ctx context.Context, q Query) (Result, error) {
	q = q.normalize()
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
		p, err := s.fetch(ctx, q)
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

// fetch asks GitHub for the page q selects and keeps what else the answer
// brought: the counts, and on a first page, the first page of every kind.
func (s *Service) fetch(ctx context.Context, q Query) (core.SearchPage[core.SearchHit], error) {
	var after map[core.SearchKind]string
	if q.Cursor != "" {
		after = map[core.SearchKind]string{q.Kind: q.Cursor}
	}
	pages, err := s.api.Search(ctx, github.SearchQuery{Text: q.Text, First: q.PageSize, After: after})
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
		if kind != q.Kind && q.Cursor == "" {
			other := q
			other.Kind = kind
			s.pages.Set(pageKey(other), cache.Entry[core.SearchPage[core.SearchHit]]{Value: page, Tags: []string{allTag}})
		}
	}
	s.addCounts(q.Text, totals)
	return p, nil
}

// all lists the first page of every kind, which the first search brings
// at once.
func (s *Service) all(ctx context.Context, q Query) (Result, error) {
	if q.Cursor != "" {
		return Result{}, fmt.Errorf("search %q: a search of every kind has one page", q.Text)
	}
	var res Result
	for _, kind := range kinds {
		k := q
		k.Kind = kind
		r, err := s.Search(ctx, k)
		if err != nil {
			return Result{}, err
		}
		res.Items = append(res.Items, r.Items...)
		res.Total += r.Total
		res.Counts = r.Counts
	}
	return res, nil
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
