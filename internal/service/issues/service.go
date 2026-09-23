// Package issues lists, shows and changes the issues of GitHub repositories.
// Reads come from a cache that is revalidated with conditional requests, and
// changes are shown before GitHub confirms them.
package issues

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// API is the part of the GitHub client that the service uses.
type API interface {
	ListIssues(ctx context.Context, repo core.RepoRef, state core.StateFilter, cursor string, cond github.Conditional) (core.Page[core.Issue], github.Response, error)
	GetIssue(ctx context.Context, repo core.RepoRef, number int, cond github.Conditional) (core.Issue, github.Response, error)
	ListIssueComments(ctx context.Context, repo core.RepoRef, number int, cursor string, cond github.Conditional) (core.Page[core.Comment], github.Response, error)
	SetIssueState(ctx context.Context, repo core.RepoRef, number int, state core.State) (core.Issue, error)
	AddIssueLabels(ctx context.Context, repo core.RepoRef, number int, names []string) ([]core.Label, error)
	RemoveIssueLabel(ctx context.Context, repo core.RepoRef, number int, name string) ([]core.Label, error)
	CreateIssueComment(ctx context.Context, repo core.RepoRef, number int, body string) (core.Comment, error)
}

// Service reads and changes issues. It is safe for concurrent use.
type Service struct {
	api    API
	viewer string
	// pending numbers the comments shown before GitHub confirms them.
	pending atomic.Uint64
	// An issue's detail is split in two entries because GitHub serves the
	// issue and its comments separately, each with its own ETag.
	lists    *cache.Cache[core.Page[core.Issue]]
	issues   *cache.Cache[core.Issue]
	comments *cache.Cache[core.Page[core.Comment]]
}

// New returns a service that calls api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return &Service{
		api:      api,
		viewer:   o.viewer,
		lists:    cache.New[core.Page[core.Issue]](o.cache...),
		issues:   cache.New[core.Issue](o.cache...),
		comments: cache.New[core.Page[core.Comment]](o.cache...),
	}
}

// fetch reads key from c, or loads it with load when it is missing or
// stale. A stale entry's validators make the request conditional.
func fetch[V any](ctx context.Context, c *cache.Cache[V], key string, tags func(V) []string,
	load func(ctx context.Context, cond github.Conditional) (V, github.Response, error),
) (V, error) {
	e, err := c.Fetch(ctx, key, func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		v, res, err := load(ctx, cond)
		if err != nil {
			return cache.Entry[V]{}, err
		}
		if res.NotModified {
			return cache.Entry[V]{}, cache.ErrNotModified
		}
		return cache.Entry[V]{Value: v, ETag: res.ETag, LastModified: res.LastModified, Tags: tags(v)}, nil
	})
	return e.Value, err
}

// Cache keys and tags. Every entry that holds an issue is tagged with the
// issue's key, so a change to the issue finds all of them.

func listKey(q ListQuery) string {
	return fmt.Sprintf("list:%s:%s:%s", q.Repo, q.State, q.Cursor)
}

func issueKey(repo core.RepoRef, number int) string {
	return fmt.Sprintf("issue:%s#%d", repo, number)
}

func commentsKey(repo core.RepoRef, number int) string {
	return fmt.Sprintf("comments:%s#%d", repo, number)
}

func repoTag(repo core.RepoRef) string {
	return "repo:" + repo.String()
}
