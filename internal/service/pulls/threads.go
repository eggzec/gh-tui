package pulls

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// The review threads of a pull request, with their comments, are read a page
// at a time for the diff and the conversation of its modal, each page cached
// under its head and cursor. Only the first page is read unless a page after
// it is asked for.
//
// GraphQL gives them no validators, so a stale page is read again in full,
// like the detail. And as for the detail, nothing vouches for a page that is
// still fresh: a thread is resolved, or answered, without the pull request's
// update time moving.

// ThreadsQuery selects a page of the review threads of a pull request.
type ThreadsQuery struct {
	Repo   core.RepoRef
	Number int
	// Head is the SHA of the pull request's head commit, as its detail shows.
	// A push is a new head, which reads its threads anew, since the lines
	// they are on and whether they are outdated change with it. It must be
	// set: without it, the pages of one push would be taken for another's.
	Head string
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many threads the page holds at most. Zero means the
	// service's size for threads, and GitHub's maximum of 100 caps it.
	PageSize int
}

// headKey is the key of the pages of the threads of the pull request at the
// head of q, apart from their cursor and size.
func (q ThreadsQuery) headKey() string {
	return "pullthreads:" + repoID(q.Repo) + "#" + strconv.Itoa(q.Number) + "@" + strings.ToLower(q.Head)
}

// key is the key of the page of q, whose size is size, the service's, unless
// q sets one.
func (q ThreadsQuery) key(size int) string {
	v := url.Values{"cursor": {q.Cursor}, "first": {strconv.Itoa(pageSize(q.PageSize, size))}}
	return q.headKey() + "?" + v.Encode()
}

// threadMarks are the Marks of a page of threads.
func threadMarks(p *core.ThreadPage) (offline, limited *bool) {
	return &p.Offline, &p.Limited
}

// CachedThreads returns the page for q if it is cached, fresh or stale,
// without fetching it.
func (s *Service) CachedThreads(q ThreadsQuery) (core.ThreadPage, bool) {
	if q.Head == "" {
		return core.ThreadPage{}, false
	}
	return cached(s.threads, q.key(s.detailSizes.Threads))
}

// Threads returns the page of the review threads of pull request q.Number at
// its head, each with its first 100 comments, in the order GitHub lists them.
// The first page, which has no Cursor, is as large as the config's
// page_size.threads, which is also how many threads the detail lists. Read
// the pages after it, one at a time, with the Next of the page before as
// Cursor. A fresh cached page is returned without a request, and what an
// earlier session kept counts as cached, as for Get. It fails if q has no
// Head.
//
// If GitHub can't be reached, a cached page is served with Offline set, and
// if it rate limits the read, with Limited set. A pull request's threads
// change as a head of it does, so a page of another head is never served.
func (s *Service) Threads(ctx context.Context, q ThreadsQuery) (core.ThreadPage, error) {
	if q.Head == "" {
		return core.ThreadPage{}, fmt.Errorf("list review threads of pull %s#%d: %w", q.Repo, q.Number, errNoHead)
	}
	key := q.key(s.detailSizes.Threads)
	s.keptThreads.Warm(s.threads, key, true)
	p, err := fetch(ctx, s.threads, s.keptThreads, key, threadMarks, whole(tags(q.Repo, q.Number), func(ctx context.Context) (core.ThreadPage, error) {
		return s.api.ListPullRequestThreads(ctx, q.Repo, q.Number, q.Cursor, pageSize(q.PageSize, s.detailSizes.Threads))
	}))
	if err != nil {
		return core.ThreadPage{}, fmt.Errorf("list review threads of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p, nil
}

// SetThreadResolved sets whether thread id of pull request number of repo is
// resolved, in every cached page that holds it, right away and without
// reading them again, for the optimistic change of a resolve or an
// unresolve. It reports false, and changes nothing, if no cached page holds
// the thread. The returned rollback undoes the change, as cache.Cache.Mutate's
// does, if GitHub refuses it. If it accepts it, ConfirmThreadResolved keeps
// the change for the next session.
//
// The viewer who may resolve a thread may also unresolve it, so the two
// permissions swap. Who resolved it isn't known until the thread is read
// again, so a resolved thread keeps none, and an unresolved one is left with
// none. Until the change is confirmed only memory has it, so a rollback
// leaves nothing behind.
func (s *Service) SetThreadResolved(repo core.RepoRef, number int, id string, resolved bool) (rollback func(), ok bool) {
	rollback = s.threads.MutateTag(detailKey(repo, number), func(p core.ThreadPage) (core.ThreadPage, bool) {
		i := slices.IndexFunc(p.Threads, func(t core.Thread) bool { return t.ID == id })
		if i < 0 {
			return p, false
		}
		// The page's slice is shared with whoever holds the cached value, so
		// the change is made on a copy.
		p.Threads = slices.Clone(p.Threads)
		t := &p.Threads[i]
		t.Resolved = resolved
		t.ResolvedBy = core.User{}
		t.ViewerCanResolve, t.ViewerCanUnresolve = !resolved, resolved
		ok = true
		return p, true
	})
	return rollback, ok
}

// ConfirmThreadResolved keeps for the next session every cached page of pull
// request number of repo whose thread id is resolved or not as resolved says,
// once GitHub has accepted the change that SetThreadResolved made. Without
// it, a page kept within the TTL would be served as it was read, with the
// thread as it was. A page keeps the time it was read, so one that was stale
// is still read again.
func (s *Service) ConfirmThreadResolved(repo core.RepoRef, number int, id string, resolved bool) {
	for key, e := range s.threads.TaggedEntries(detailKey(repo, number)) {
		held := slices.ContainsFunc(e.Value.Threads, func(t core.Thread) bool { return t.ID == id && t.Resolved == resolved })
		if !held {
			continue
		}
		_ = s.keptThreads.Save(key, cache.Entry[core.ThreadPage]{Value: e.Value, Tags: e.Tags, FetchedAt: e.FetchedAt})
	}
}
