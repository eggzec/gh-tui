package pulls

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// Kept lists the list and comment pages that the service keeps with
// validators, for a revalidator to check in the background. Each check is
// one conditional request, which costs no rate limit when nothing changed.
// A list page is a GraphQL read, which has no validators, so it carries
// the probe's ETag (see loadList) and the check only confirms it with the
// probe. A page the probe can't confirm is skipped without a request. A
// page the probe finds changed isn't read in full, since that would spend
// the GraphQL quota on repositories the user may not look at: the check
// reports SyncKey of its repository, so that the views showing it read it
// again, and counts as done, since the probe that found the change was a
// request. A comment page that changed is cached and kept, and reports
// SyncKey of its repository, so that the views showing it read it again.
// The details have no validators: Poll and the list's update times watch
// them instead. It reads the store, so call it where I/O is fine.
func (s *Service) Kept() []revalidate.Entry {
	return slices.Concat(
		recheck.Entries(s.keptLists, kindList, s.ttl, s.listTarget),
		recheck.Entries(s.keptComments, kindComments, s.ttl, s.commentsTarget),
	)
}

// errUnconfirmed is a list page that the probe can't vouch for, so it
// wasn't sent.
var errUnconfirmed = errors.New("page not confirmed by the probe")

// errProbeChanged is a list page that the probe found changed.
var errProbeChanged = errors.New("page changed since the probe vouched for it")

func (s *Service) listTarget(key string) (recheck.Target, bool) {
	q, ok := parseListKey(key, s.pageSize)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		e, res := recheck.Check(ctx, s.lists, s.keptLists, key, SyncKey(q.Repo), s.confirmList(q))
		switch {
		case errors.Is(res.Err, errUnconfirmed):
			return revalidate.Result{Status: revalidate.Skipped}
		case errors.Is(res.Err, errProbeChanged):
			// Unlike a skip, the probe was a request that answered 200, and
			// cost a point of the REST rate limit. Reporting it as a change
			// spends the revalidator's budget on it, leaves the page alone
			// until it is due again, and has the views read the page.
			return revalidate.Result{Status: revalidate.Changed, Sync: SyncKey(q.Repo)}
		case res.Status == revalidate.NotModified:
			// As a read of the page does, it vouches for what is cached
			// of its pull requests.
			s.vouch(q.Repo, e.Value.Page.Items)
		}
		return res
	}}, true
}

// confirmList returns a load for fetch that confirms the cached page of q
// with the probe, and never reads it: it fails with errUnconfirmed for a
// page the probe can't vouch for, and with errProbeChanged for one it finds
// changed.
func (s *Service) confirmList(q ListQuery) cache.FetchFunc[listPage] {
	return func(ctx context.Context, prev cache.Entry[listPage], ok bool) (cache.Entry[listPage], error) {
		if !ok || prev.ETag == "" || s.unconfirmable(q.Repo, prev.Value) != "" {
			return cache.Entry[listPage]{}, errUnconfirmed
		}
		res, err := s.api.ProbePullRequests(ctx, q.Repo, github.Conditional{ETag: prev.ETag})
		switch {
		case err != nil:
			return cache.Entry[listPage]{}, err
		case !res.NotModified:
			return cache.Entry[listPage]{}, errProbeChanged
		}
		return cache.Entry[listPage]{}, cache.ErrNotModified
	}
}

// parseListKey returns the query that ListQuery.key made key of with size,
// the service's page size, for a page without a filter, the only ones
// kept. The repository comes back in lower case, which GitHub doesn't
// mind.
func parseListKey(key string, size int) (ListQuery, bool) {
	name, query, ok := strings.Cut(strings.TrimPrefix(key, "pulls:"), "?")
	if !ok || !strings.HasPrefix(key, "pulls:") {
		return ListQuery{}, false
	}
	repo, err := core.ParseRepoRef(name)
	v, verr := url.ParseQuery(query)
	if err != nil || verr != nil || v.Has("filter") {
		return ListQuery{}, false
	}
	first, err := strconv.Atoi(v.Get("first"))
	if err != nil {
		return ListQuery{}, false
	}
	q := ListQuery{Repo: repo, State: core.State(v.Get("state")), Cursor: v.Get("cursor"), PageSize: first}
	// A key that doesn't make itself again isn't one of the service's.
	return q, q.key(size) == key
}

func (s *Service) commentsTarget(key string) (recheck.Target, bool) {
	q, ok := parseCommentsKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		// As for Comments, the page is at least as recent as what the
		// list showed before the request.
		m, _ := s.seen.Get(detailKey(q.Repo, q.Number))
		_, res := recheck.Check(ctx, s.comments, s.keptComments, key, SyncKey(q.Repo), s.loadComments(q, m.updated))
		return res
	}}, true
}

// parseCommentsKey returns the query that CommentsQuery.key made key of.
// The repository comes back in lower case, which GitHub doesn't mind.
func parseCommentsKey(key string) (CommentsQuery, bool) {
	pull, query, ok := strings.Cut(strings.TrimPrefix(key, "pull:"), "/comments?")
	if !ok || !strings.HasPrefix(key, "pull:") {
		return CommentsQuery{}, false
	}
	name, num, ok := strings.Cut(pull, "#")
	if !ok {
		return CommentsQuery{}, false
	}
	repo, err := core.ParseRepoRef(name)
	number, nerr := strconv.Atoi(num)
	v, verr := url.ParseQuery(query)
	if err != nil || nerr != nil || verr != nil || number <= 0 {
		return CommentsQuery{}, false
	}
	size, err := strconv.Atoi(v.Get("first"))
	if err != nil || size <= 0 {
		return CommentsQuery{}, false
	}
	q := CommentsQuery{Repo: repo, Number: number, Cursor: v.Get("cursor"), PageSize: size}
	// A key that doesn't make itself again isn't one of the service's.
	return q, q.key(size) == key
}
