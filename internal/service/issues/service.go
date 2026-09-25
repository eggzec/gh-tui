// Package issues lists, shows and changes the issues of GitHub repositories.
// Reads come from a cache that is revalidated with conditional requests, and
// changes are shown before GitHub confirms them.
package issues

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/probe"
	"github.com/eggzec/gh-tui/internal/service/seen"
)

// API is the part of the GitHub client that the service uses. ProbeIssues
// is a cheap conditional request that Poll watches.
type API interface {
	ListIssues(ctx context.Context, repo core.RepoRef, state core.StateFilter, cursor string, perPage int, cond github.Conditional) (core.Page[core.Issue], github.Response, error)
	FilterIssues(ctx context.Context, repo core.RepoRef, f github.IssueFilter, cursor string, perPage int, cond github.Conditional) (core.Page[core.Issue], github.Response, error)
	SearchIssues(ctx context.Context, query, cursor string, perPage int) (core.Page[core.SearchHit], error)
	ViewerLogin(ctx context.Context) (string, error)
	GetIssue(ctx context.Context, repo core.RepoRef, number int, cond github.Conditional) (core.Issue, github.Response, error)
	ListIssueComments(ctx context.Context, repo core.RepoRef, number int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Comment], github.Response, error)
	SetIssueState(ctx context.Context, repo core.RepoRef, number int, state core.State) (core.Issue, error)
	AddIssueLabels(ctx context.Context, repo core.RepoRef, number int, names []string) ([]core.Label, error)
	RemoveIssueLabel(ctx context.Context, repo core.RepoRef, number int, name string) ([]core.Label, error)
	CreateIssueComment(ctx context.Context, repo core.RepoRef, number int, body string) (core.Comment, error)
	ProbeIssues(ctx context.Context, repo core.RepoRef, cond github.Conditional) (github.Response, error)
}

// Service reads and changes issues. It is safe for concurrent use.
type Service struct {
	api    API
	viewer string
	// pending numbers the comments shown before GitHub confirms them.
	pending atomic.Uint64
	// An issue and each page of its comments are separate entries, each with
	// its own ETag, so the tui can page through a long thread and keep only
	// the pages it shows. Comment pages carry the version of the issue they
	// were read at.
	lists    *cache.Cache[core.Page[core.Issue]]
	issues   *cache.Cache[core.Issue]
	comments *cache.Cache[stampedComments]
	// The kept ones are what an earlier session read of each, if the
	// service has a store. A read that misses memory starts from them.
	keptLists    *cache.Shelf[core.Page[core.Issue]]
	keptIssues   *cache.Shelf[core.Issue]
	keptComments *cache.Shelf[stampedComments]
	// etags holds the latest probe ETag of each polled repository.
	etags probe.Tracker
	// seen holds when each issue last changed, as the list pages last
	// showed, by issue key.
	seen seen.Marks[time.Time]
	// me is the login that @me stands for in a filtered list, read once
	// when viewer isn't set.
	meMu sync.Mutex
	me   string
}

// stampedComments is a cached page of comments with the version of its issue.
type stampedComments = seen.Stamped[core.Page[core.Comment]]

// New returns a service that calls api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	s := &Service{
		api:          api,
		viewer:       o.viewer,
		lists:        cache.New[core.Page[core.Issue]](o.cache...),
		issues:       cache.New[core.Issue](o.cache...),
		comments:     cache.New[stampedComments](o.cache...),
		keptLists:    cache.NewShelf[core.Page[core.Issue]](o.store, kindList, schema),
		keptIssues:   cache.NewShelf[core.Issue](o.store, kindIssue, schema),
		keptComments: cache.NewShelf[stampedComments](o.store, kindComments, schema),
	}
	s.etags.Keep(o.store)
	return s
}

// The kinds of entries the service keeps in its store, and the version of
// their values. Bump schema when core.Issue or core.Comment change shape.
const (
	kindList     = "issuelist"
	kindIssue    = "issue"
	kindComments = "issuecomments"
	schema       = 3
)

// offlineAt is when an entry served offline was fetched, as far as the
// cache can tell: long ago, so it is stale at once and the next read asks
// GitHub again.
var offlineAt = time.Unix(1, 0)

// fetch reads key from c, or loads it with load when it is missing or
// stale. A stale entry's validators make the request conditional. What
// GitHub sends is kept on shelf too, and what GitHub refuses is dropped from
// it. If GitHub can't be reached, the stale entry is served instead, marked
// by offline.
func fetch[V any](ctx context.Context, c *cache.Cache[V], shelf *cache.Shelf[V], key string, tags func(V) []string, offline func(V) V,
	load func(ctx context.Context, cond github.Conditional) (V, github.Response, error),
) (V, error) {
	e, err := c.Fetch(ctx, key, func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		v, res, err := load(ctx, cond)
		switch {
		case ok && github.Unreachable(ctx, err):
			prev.Value, prev.FetchedAt = offline(prev.Value), offlineAt
			return prev, nil
		case err != nil:
			if github.Refused(err) {
				shelf.Delete(key)
			}
			return cache.Entry[V]{}, err
		case res.NotModified:
			// The kept entry, if any, is the one revalidated, and prev
			// may hold changes GitHub hasn't confirmed, so it isn't
			// kept again.
			return cache.Entry[V]{}, cache.ErrNotModified
		}
		e := cache.Entry[V]{Value: v, ETag: res.ETag, LastModified: res.LastModified, Source: res.URL, Tags: tags(v)}
		// The shelf is only a shortcut, so a failure is ignored.
		_ = shelf.Save(key, e)
		return e, nil
	})
	return e.Value, err
}

// offlinePage marks a page served offline.
func offlinePage[T any](p core.Page[T]) core.Page[T] {
	p.Offline = true
	return p
}

// asIs serves a value offline unmarked.
func asIs[V any](v V) V { return v }

// Cache keys and tags. Every entry that holds an issue, or comments on it,
// is tagged with the issue's key, so a change to the issue finds all of them.

func listKey(q ListQuery) string {
	if q.Filter != "" {
		// Filtered lists aren't kept, so their keys needn't parse back.
		return fmt.Sprintf("filtered:%s:%s:%d:%q:%s", q.Repo, q.State, q.PageSize, q.Filter, q.Cursor)
	}
	return fmt.Sprintf("list:%s:%s:%d:%s", q.Repo, q.State, q.PageSize, q.Cursor)
}

func issueKey(repo core.RepoRef, number int) string {
	return issuePrefix(repo) + strconv.Itoa(number)
}

// issuePrefix starts the key of every issue of repo.
func issuePrefix(repo core.RepoRef) string {
	return "issue:" + repo.String() + "#"
}

func commentsKey(q CommentsQuery) string {
	return fmt.Sprintf("comments:%s#%d:%d:%s", q.Repo, q.Number, q.PageSize, q.Cursor)
}

func repoTag(repo core.RepoRef) string {
	return "repo:" + repo.String()
}
