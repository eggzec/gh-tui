// Package issues lists, shows and changes the issues of GitHub repositories.
// Reads come from a cache that is revalidated with conditional requests, and
// changes are shown before GitHub confirms them.
package issues

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/internal/service/probe"
	"github.com/eggzec/gh-tui/internal/service/recheck"
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
	GetIssueKind(ctx context.Context, repo core.RepoRef, number int, cond github.Conditional) (core.NumberKind, core.Issue, github.Response, error)
	ListIssueComments(ctx context.Context, repo core.RepoRef, number int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Comment], github.Response, error)
	SetIssueState(ctx context.Context, repo core.RepoRef, number int, state core.State) (core.Issue, error)
	AddIssueLabels(ctx context.Context, repo core.RepoRef, number int, names []string) ([]core.Label, error)
	RemoveIssueLabel(ctx context.Context, repo core.RepoRef, number int, name string) ([]core.Label, error)
	CreateIssueComment(ctx context.Context, repo core.RepoRef, number int, body string) (core.Comment, error)
	ProbeIssues(ctx context.Context, repo core.RepoRef, cond github.Conditional) (github.Response, error)
}

// Access tells whether the token may do what an operation needs, as the
// access service does: nil, or why not.
type Access interface {
	Check(n core.Need) error
}

// Repos holds the repositories read so far, as the repositories service
// does, with what the viewer may do in each.
type Repos interface {
	CachedGet(ref core.RepoRef) (core.Repo, bool)
}

// Service reads and changes issues. It is safe for concurrent use.
type Service struct {
	api API
	// access refuses a change the token may not make before it is shown,
	// if set, and repos tells whether a repository is private, for it.
	access Access
	repos  Repos
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
	// kinds holds whether each number resolved so far is an issue or a pull
	// request, which never changes, so it is kept for good, and on
	// keptKinds without validators. pulls tells a pull request from its
	// cached detail, if set.
	kinds     kindMemo
	keptKinds *cache.Shelf[core.NumberKind]
	pulls     PullCache
	// etags holds the latest probe ETag of each polled repository.
	etags probe.Tracker
	// seen holds when each issue last changed, as the list pages last
	// showed, by issue key.
	seen seen.Marks[time.Time]
	// me is the login that @me stands for in a filtered list, read once
	// when viewer isn't set.
	meMu sync.Mutex
	me   string
	// pageSize is the size of a page whose query sets none.
	pageSize int
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
		access:       o.access,
		repos:        o.repos,
		lists:        cache.New[core.Page[core.Issue]](o.cache...),
		issues:       cache.New[core.Issue](o.cache...),
		comments:     cache.New[stampedComments](o.cache...),
		keptLists:    cache.NewShelf[core.Page[core.Issue]](o.store, kindList, schema),
		keptIssues:   cache.NewShelf[core.Issue](o.store, kindIssue, schema),
		keptComments: cache.NewShelf[stampedComments](o.store, kindComments, schema),
		keptKinds:    cache.NewShelf[core.NumberKind](o.store, kindNumber, numberSchema),
		pulls:        o.pulls,
		pageSize:     cmp.Or(o.pageSize, config.Default().PageSize.Issues),
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
	// A number's kind is kept apart, since it outlives any shape of
	// core.Issue.
	kindNumber   = "numberkind"
	numberSchema = 1
)

// stampedMarks are the Marks of a stamped page of comments.
func stampedMarks(p *stampedComments) (offline, limited *bool) {
	return fallback.Page(&p.Value)
}

// fetch reads key from c, or loads it with load when it is missing or
// stale, falling back on the stale entry as fallback.Fetch does, marked by
// marks. A stale entry's validators make the request conditional. What
// GitHub sends is kept on shelf too.
func fetch[V any](ctx context.Context, c *cache.Cache[V], shelf *cache.Shelf[V], key string, tags func(V) []string, marks fallback.Marks[V],
	load func(ctx context.Context, cond github.Conditional) (V, github.Response, error),
) (V, error) {
	e, err := fallback.Fetch(ctx, c, shelf, key, marks, fallback.Keep(shelf, key, recheck.Load(load, tags)))
	return e.Value, err
}

// Cache keys and tags. Every entry that holds an issue, or comments on it,
// is tagged with the issue's key, so a change to the issue finds all of them.

func listKey(q ListQuery) string {
	if q.Filter != "" {
		// Filtered lists aren't kept, so their keys needn't parse back.
		// The filter comes after '?', which a record of the key leaves
		// out, since the user typed it.
		return fmt.Sprintf("filtered:%s:%s:%d:%s?filter=%s", repoID(q.Repo), q.State, q.PageSize, q.Cursor, url.QueryEscape(q.Filter))
	}
	return fmt.Sprintf("list:%s:%s:%d:%s", repoID(q.Repo), q.State, q.PageSize, q.Cursor)
}

func issueKey(repo core.RepoRef, number int) string {
	return issuePrefix(repo) + strconv.Itoa(number)
}

// issuePrefix starts the key of every issue of repo.
func issuePrefix(repo core.RepoRef) string {
	return "issue:" + repoID(repo) + "#"
}

func commentsKey(q CommentsQuery) string {
	return fmt.Sprintf("comments:%s#%d:%d:%s", repoID(q.Repo), q.Number, q.PageSize, q.Cursor)
}

func repoTag(repo core.RepoRef) string {
	return "repo:" + repoID(repo)
}

// repoID names a repository in keys and tags. GitHub ignores case in owner
// and repository names, so keys do too.
func repoID(r core.RepoRef) string {
	return strings.ToLower(r.String())
}
