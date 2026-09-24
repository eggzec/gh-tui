// Package issues lists, shows and changes the issues of GitHub repositories.
// Reads come from a cache that is revalidated with conditional requests, and
// changes are shown before GitHub confirms them.
package issues

import (
	"context"
	"fmt"
	"strconv"
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
	// etags holds the latest probe ETag of each polled repository.
	etags probe.Tracker
	// seen holds when each issue last changed, as the list pages last
	// showed, by issue key.
	seen seen.Marks[time.Time]
}

// stampedComments is a cached page of comments with the version of its issue.
type stampedComments = seen.Stamped[core.Page[core.Comment]]

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
		comments: cache.New[stampedComments](o.cache...),
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

// Cache keys and tags. Every entry that holds an issue, or comments on it,
// is tagged with the issue's key, so a change to the issue finds all of them.

func listKey(q ListQuery) string {
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
