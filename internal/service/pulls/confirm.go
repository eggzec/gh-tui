package pulls

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// A list page is a GraphQL read, which has no validators, so reading a stale
// page again would cost a full query. Instead each page carries, as its
// ETag, that of the probe Poll sends, from before the page was read: a
// conditional request for the most recently updated pull request of the
// repository, which GitHub answers with a free 304 while no pull request
// changed. When it does, the page is still what GitHub would send, and it
// is only marked fetched now.
//
// A page read for the first time doesn't wait for a probe: it takes the
// ETag of Poll's latest one, if any. A page without an ETag, such as one
// read before any probe, or kept by a version that didn't probe, is read
// again after a probe of its own, which costs one request of the REST rate
// limit per page on its first revisit. That probe's ETag isn't given to
// Poll, since Poll would then miss the changes it stands for.
//
// The probe doesn't see everything: GitHub doesn't count checks as
// updates, a renamed label changes no pull request, and REST and GraphQL
// may read replicas that lag each other. So a page is only confirmed for a
// while after it was last read in full, and never after the user asked for
// a refresh, nor while it shows checks running. Failed checks are often
// run again, so a page that shows them is confirmed for a shorter while:
// failingFor TTLs.

// confirmFor is how long after a page was last read in full the probe may
// still confirm it: past it, what the probe can't see is read again. It is
// a few times the default TTL, so that a busy repository still costs one
// full read in half an hour.
const confirmFor = 30 * time.Minute

// failingFor is how many TTLs after a page was last read in full the probe
// may still confirm it while it shows failed checks. A re-run doesn't move
// the update time, so this bounds how long a re-run that passed goes
// unseen, while a page that shows a failure, which most busy repositories
// have, still costs no full read on every visit.
const failingFor = 2

// listPage is a cached list page with when GitHub last sent it in full,
// which a probe that confirms it doesn't move.
type listPage struct {
	Page   core.Page[core.PullRequest]
	ReadAt time.Time
}

// refreshes holds when the user last asked for each repository to be read
// again, by repository ID.
type refreshes struct {
	mu sync.Mutex
	at map[string]time.Time
}

func (r *refreshes) set(repo core.RepoRef, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.at == nil {
		r.at = make(map[string]time.Time)
	}
	r.at[repoID(repo)] = at
}

func (r *refreshes) get(repo core.RepoRef) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.at[repoID(repo)]
}

// loadList returns a load for fetch that reads the page of q, unless a probe
// finds the cached page still current.
func (s *Service) loadList(q ListQuery) cache.FetchFunc[listPage] {
	return func(ctx context.Context, prev cache.Entry[listPage], ok bool) (cache.Entry[listPage], error) {
		etag, err := s.probeList(ctx, q, prev, ok)
		if err != nil {
			return cache.Entry[listPage]{}, err
		}
		readAt := s.now()
		p, err := s.readList(ctx, q)
		if err != nil {
			return cache.Entry[listPage]{}, err
		}
		return cache.Entry[listPage]{Value: listPage{Page: p, ReadAt: readAt}, ETag: etag, Tags: []string{repoTag(q.Repo)}}, nil
	}
}

// probeList asks whether prev, the cached page of q if ok, is still
// current. It returns cache.ErrNotModified if it is, and otherwise the ETag
// that the page read next is to carry, if any.
func (s *Service) probeList(ctx context.Context, q ListQuery, prev cache.Entry[listPage], ok bool) (string, error) {
	if !probed(q) {
		return "", nil
	}
	// The latest probe predates the read that follows, so its ETag can
	// vouch for it later.
	latest := s.etags.ETag(SyncKey(q.Repo))
	if !ok {
		return latest, nil
	}
	etag, found := prev.ETag, s.unconfirmable(q.Repo, prev.Value)
	if etag == "" {
		// There is nothing to confirm, but the page is being read again,
		// so the probe runs before it. With Poll's ETag it may be free.
		etag, found = latest, "unstamped"
	}
	res, err := s.api.ProbePullRequests(ctx, q.Repo, github.Conditional{ETag: etag})
	switch {
	case github.Unreachable(ctx, err) || github.Refused(err):
		return "", err
	case err != nil:
		// Such as the REST rate limit, which the GraphQL read doesn't
		// share: the page is read without a probe to vouch for it.
		slog.WarnContext(ctx, "list not probed", "span", "service.pulls", "repo", q.Repo.String(), "err", err.Error())
		return "", nil
	case !res.NotModified:
		etag, found = res.ETag, "changed"
	case found == "":
		found, err = "not_modified", cache.ErrNotModified
	}
	slog.InfoContext(ctx, "list probed", "span", "service.pulls", "repo", q.Repo.String(), "found", found)
	return etag, err
}

// unconfirmable returns why a 304 can't confirm p, a page of repo, or ""
// if it can.
func (s *Service) unconfirmable(repo core.RepoRef, p listPage) string {
	switch {
	case !p.ReadAt.After(s.refreshes.get(repo)):
		// A read at the instant of the refresh may predate it.
		return "refreshed"
	case s.now().Sub(p.ReadAt) >= confirmFor:
		return "old"
	case has(p.Page.Items, core.ChecksPending):
		return "checks running"
	case has(p.Page.Items, core.ChecksFailure) && s.now().Sub(p.ReadAt) >= failingFor*s.ttl:
		return "checks failed"
	default:
		return ""
	}
}

// probed reports whether the probe vouches for the page of q: those the
// repository's list reads. A search may not show a change yet when the
// probe does, so its pages are read again in full.
func probed(q ListQuery) bool {
	if q.Filter == "" {
		return true
	}
	_, ok := listFilter(q)
	return ok
}

// has reports whether any of prs has checks in state.
func has(prs []core.PullRequest, state core.ChecksState) bool {
	return slices.ContainsFunc(prs, func(pr core.PullRequest) bool { return pr.Checks == state })
}
