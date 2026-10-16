package pulls

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// The commits of a pull request with their check states, and the viewer's
// last review, are read in one request under the head of the pull request.
// The states change while checks run and the review changes on the web, so
// they are kept in memory only, fresh for the pull requests' TTL, and read
// again with the detail: wherever a refresh drops the detail, it drops them.

// PullCommitsQuery selects the commits of a pull request with their checks.
type PullCommitsQuery struct {
	Repo   core.RepoRef
	Number int
	// Head is the SHA of the pull request's head commit, as its detail
	// shows. A push is a new head, which reads the commits anew. It must be
	// set: without it, the commits of one push would be taken for another's.
	Head string
}

// key is the key of the commits of q.
func (q PullCommitsQuery) key() string {
	return "pullcommitchecks:" + repoID(q.Repo) + "#" + strconv.Itoa(q.Number) + "@" + strings.ToLower(q.Head)
}

// CachedPullCommits returns the commits of q if they are cached, fresh or
// stale, without fetching them.
func (s *Service) CachedPullCommits(q PullCommitsQuery) (core.PullCommits, bool) {
	if q.Head == "" {
		return core.PullCommits{}, false
	}
	return cached(s.pullCommits, q.key())
}

// PullCommits returns the commits of pull request q.Number, oldest first, each
// with the state of its checks, and the last review the signed-in user
// submitted on it. A fresh cached read is returned without a request. GitHub
// lists core.MaxPullCommits commits at most, and Capped says that there are
// more. It fails if q has no Head.
//
// The read is marked stale by what makes the detail read again: a refresh
// (Invalidate, Revalidate), a change a probe found, and a review the user
// submitted.
func (s *Service) PullCommits(ctx context.Context, q PullCommitsQuery) (core.PullCommits, error) {
	if q.Head == "" {
		return core.PullCommits{}, fmt.Errorf("list commits of pull %s#%d: %w", q.Repo, q.Number, errNoHead)
	}
	e, err := s.pullCommits.Fetch(ctx, q.key(), func(ctx context.Context, _ cache.Entry[core.PullCommits], _ bool) (cache.Entry[core.PullCommits], error) {
		viewer, err := s.viewerLogin(ctx)
		if err != nil {
			return cache.Entry[core.PullCommits]{}, err
		}
		pc, err := s.api.ListPullCommits(ctx, q.Repo, q.Number, viewer)
		if err != nil {
			return cache.Entry[core.PullCommits]{}, err
		}
		return cache.Entry[core.PullCommits]{Value: pc, Tags: tags(q.Repo, q.Number)}, nil
	})
	if err != nil {
		return core.PullCommits{}, fmt.Errorf("list commits of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return e.Value, nil
}

// viewerLogin returns the login of the signed-in user, which it reads once. A
// failure is not kept: the next read asks again.
func (s *Service) viewerLogin(ctx context.Context) (string, error) {
	s.viewerMu.Lock()
	defer s.viewerMu.Unlock()
	if s.viewer != "" {
		return s.viewer, nil
	}
	login, err := s.api.ViewerLogin(ctx)
	if err != nil {
		return "", err
	}
	s.viewer = login
	return login, nil
}
