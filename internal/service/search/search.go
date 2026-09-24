// Package search finds repositories, issues and pull requests on GitHub and
// keeps the results briefly, so typing a query again, or going back to it,
// costs no request.
package search

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// Defaults of a Service and a Query.
const (
	// DefaultPageSize is the page size of a Query that sets none. A search
	// of every kind fetches a page of this size of each.
	DefaultPageSize = 20
	// DefaultTTL is short: results change, and a query typed again soon
	// after is the case the cache is for.
	DefaultTTL = 30 * time.Second
	// DefaultCapacity is how many pages of results a Service keeps.
	DefaultCapacity = 256
)

// maxPageSize is the largest page GitHub returns.
const maxPageSize = 100

// API is the part of the GitHub client the service uses.
type API interface {
	SearchRepos(ctx context.Context, query, cursor string, perPage int) (core.Page[core.Repo], error)
	SearchIssues(ctx context.Context, query, cursor string, perPage int) (core.Page[core.SearchHit], error)
}

// Query selects a page of search results.
type Query struct {
	// Text is the query in GitHub's search syntax, qualifiers included.
	Text string
	// Kind limits the results to repositories, issues or pull requests.
	// The zero value, core.SearchAll, lists repositories first, then
	// issues and pull requests.
	Kind core.SearchKind
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many results of each kind a page holds. Zero means
	// DefaultPageSize, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
}

func (q Query) normalize() Query {
	q.Text = strings.Join(strings.Fields(q.Text), " ")
	if q.PageSize <= 0 {
		q.PageSize = DefaultPageSize
	}
	q.PageSize = min(q.PageSize, maxPageSize)
	return q
}

// Service searches GitHub through a cache. It is safe for concurrent use.
type Service struct {
	api   API
	pages *cache.Cache[core.Page[core.SearchHit]]
}

// New returns a Service that searches with api.
func New(api API, opts ...Option) *Service {
	o := options{ttl: DefaultTTL, capacity: DefaultCapacity}
	for _, opt := range opts {
		opt(&o)
	}
	return &Service{
		api:   api,
		pages: cache.New[core.Page[core.SearchHit]](cache.WithTTL(o.ttl), cache.WithCapacity(o.capacity)),
	}
}

// CachedSearch returns the cached page for q, fresh or stale, without I/O.
// A query without text has no results, which are always known.
func (s *Service) CachedSearch(q Query) (core.Page[core.SearchHit], bool) {
	q = q.normalize()
	if q.Text == "" {
		return core.Page[core.SearchHit]{}, true
	}
	e, state := s.pages.Get(key(q))
	return e.Value, state != cache.Miss
}

// Search returns the page for q. A fresh cached page is returned without a
// request, and a query without text returns an empty page without one.
// GitHub allows 30 searches a minute; beyond that Search fails with an
// error matching core.ErrRateLimited.
func (s *Service) Search(ctx context.Context, q Query) (core.Page[core.SearchHit], error) {
	q = q.normalize()
	if q.Text == "" {
		return core.Page[core.SearchHit]{}, nil
	}
	e, err := s.pages.Fetch(ctx, key(q), func(ctx context.Context, _ cache.Entry[core.Page[core.SearchHit]], _ bool) (cache.Entry[core.Page[core.SearchHit]], error) {
		// Search responses carry no validators worth keeping, so a stale
		// page is fetched again in full.
		p, err := s.fetch(ctx, q)
		if err != nil {
			return cache.Entry[core.Page[core.SearchHit]]{}, err
		}
		return cache.Entry[core.Page[core.SearchHit]]{Value: p, Tags: []string{allTag}}, nil
	})
	if err != nil {
		return core.Page[core.SearchHit]{}, fmt.Errorf("search %q: %w", q.Text, err)
	}
	return e.Value, nil
}

// Invalidate marks every cached page stale, so the next search of each goes
// to GitHub.
func (s *Service) Invalidate() {
	s.pages.InvalidateTag(allTag)
}

func (s *Service) fetch(ctx context.Context, q Query) (core.Page[core.SearchHit], error) {
	switch q.Kind {
	case core.SearchRepos:
		return s.repos(ctx, q.Text, q.Cursor, q.PageSize)
	case core.SearchIssues:
		return s.api.SearchIssues(ctx, q.Text+" is:issue", q.Cursor, q.PageSize)
	case core.SearchPulls:
		return s.api.SearchIssues(ctx, q.Text+" is:pr", q.Cursor, q.PageSize)
	default:
		return s.all(ctx, q)
	}
}

func (s *Service) repos(ctx context.Context, text, cursor string, perPage int) (core.Page[core.SearchHit], error) {
	p, err := s.api.SearchRepos(ctx, text, cursor, perPage)
	if err != nil {
		return core.Page[core.SearchHit]{}, err
	}
	hits := make([]core.SearchHit, len(p.Items))
	for i := range p.Items {
		hits[i] = core.SearchHit{Kind: core.SearchRepos, Repo: p.Items[i]}
	}
	return core.Page[core.SearchHit]{Items: hits, Next: p.Next}, nil
}

// all searches repositories and issues at once, one page of each. Its
// cursor holds the next cursor of both searches, and a search that has no
// more pages drops out of it.
func (s *Service) all(ctx context.Context, q Query) (core.Page[core.SearchHit], error) {
	cur := allCursor{repos: true, issues: true}
	if q.Cursor != "" {
		var err error
		if cur, err = parseAllCursor(q.Cursor); err != nil {
			return core.Page[core.SearchHit]{}, err
		}
	}

	var (
		wg            sync.WaitGroup
		repos, issues core.Page[core.SearchHit]
		rErr, iErr    error
	)
	if cur.repos {
		wg.Go(func() { repos, rErr = s.repos(ctx, q.Text, cur.reposCursor, q.PageSize) })
	}
	if cur.issues {
		wg.Go(func() { issues, iErr = s.api.SearchIssues(ctx, q.Text, cur.issuesCursor, q.PageSize) })
	}
	wg.Wait()
	if rErr != nil {
		return core.Page[core.SearchHit]{}, rErr
	}
	if iErr != nil {
		return core.Page[core.SearchHit]{}, iErr
	}

	items := make([]core.SearchHit, 0, len(repos.Items)+len(issues.Items))
	items = append(items, repos.Items...)
	items = append(items, issues.Items...)
	next := allCursor{
		repos: repos.Next != "", reposCursor: repos.Next,
		issues: issues.Next != "", issuesCursor: issues.Next,
	}
	return core.Page[core.SearchHit]{Items: items, Next: next.String()}, nil
}

// allCursor is the cursor of a search of every kind: whether each search
// has more pages, and the cursor of its next one. An empty cursor on a
// search that has more asks for its first page.
type allCursor struct {
	repos, issues             bool
	reposCursor, issuesCursor string
}

// String encodes the cursor, or returns "" when neither search has more.
func (c allCursor) String() string {
	v := url.Values{}
	if c.repos {
		v.Set("repos", c.reposCursor)
	}
	if c.issues {
		v.Set("issues", c.issuesCursor)
	}
	return v.Encode()
}

func parseAllCursor(s string) (allCursor, error) {
	v, err := url.ParseQuery(s)
	if err != nil {
		return allCursor{}, fmt.Errorf("parse cursor: %w", err)
	}
	return allCursor{
		repos:        v.Has("repos"),
		reposCursor:  v.Get("repos"),
		issues:       v.Has("issues"),
		issuesCursor: v.Get("issues"),
	}, nil
}

// allTag marks every entry, so Invalidate finds them all.
const allTag = "all"

// GitHub matches search terms and qualifiers without regard to case, so
// keys do too.
func key(q Query) string {
	return "search?kind=" + string(q.Kind) + "&page_size=" + strconv.Itoa(q.PageSize) +
		"&cursor=" + url.QueryEscape(q.Cursor) + "&q=" + url.QueryEscape(strings.ToLower(q.Text))
}
