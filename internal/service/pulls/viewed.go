package pulls

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// The files of a pull request that the viewer marked viewed are kept by GitHub
// under the pull request's head, and the mark shows on the web and on every
// machine. They are read in memory only, since the web changes them and a read
// costs about a point: fresh for the pull requests' TTL, and read again with
// the detail, as the commits are.

// viewedAPI is the part of the GitHub client that reads and sets the marks.
type viewedAPI interface {
	// ListPullViewed returns the viewer's viewed marks on the files of a pull
	// request.
	ListPullViewed(ctx context.Context, repo core.RepoRef, number int) (core.PullViewed, error)
	// SetFileViewed marks the file at path of the pull request with node ID
	// pullID viewed, or unmarks it.
	SetFileViewed(ctx context.Context, pullID, path string, viewed bool) error
}

// ViewedQuery selects the viewed marks of a pull request's files.
type ViewedQuery struct {
	Repo   core.RepoRef
	Number int
	// Head is the SHA of the pull request's head commit, as its detail shows.
	// GitHub keeps the marks per head, so a push is a read anew. It must be
	// set: without it, the marks of one push would be taken for another's.
	Head string
}

// key is the key of the marks of q.
func (q ViewedQuery) key() string {
	return "pullviewed:" + repoID(q.Repo) + "#" + strconv.Itoa(q.Number) + "@" + strings.ToLower(q.Head)
}

// CachedViewed returns the marks of q if they are cached, fresh or stale,
// without fetching them.
func (s *Service) CachedViewed(q ViewedQuery) (core.PullViewed, bool) {
	if q.Head == "" {
		return core.PullViewed{}, false
	}
	return cached(s.pullViewed, q.key())
}

// Viewed returns the viewer's viewed marks on the files of pull request
// q.Number. A fresh cached read is returned without a request. GitHub lists
// core.MaxPullFiles files at most, and Capped says that there are more. It
// fails if q has no Head.
//
// The read is marked stale by what makes the detail read again: a refresh
// (Invalidate, Revalidate) and a change a probe found.
func (s *Service) Viewed(ctx context.Context, q ViewedQuery) (core.PullViewed, error) {
	if q.Head == "" {
		return core.PullViewed{}, fmt.Errorf("list viewed files of pull %s#%d: %w", q.Repo, q.Number, errNoHead)
	}
	e, err := s.pullViewed.Fetch(ctx, q.key(), func(ctx context.Context, _ cache.Entry[core.PullViewed], _ bool) (cache.Entry[core.PullViewed], error) {
		v, err := s.api.ListPullViewed(ctx, q.Repo, q.Number)
		if err != nil {
			return cache.Entry[core.PullViewed]{}, err
		}
		return cache.Entry[core.PullViewed]{Value: v, Tags: tags(q.Repo, q.Number)}, nil
	})
	if err != nil {
		return core.PullViewed{}, fmt.Errorf("list viewed files of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return e.Value, nil
}

// SetViewed marks the file at path of pull request q.Number viewed, or
// unmarks it if viewed is false. The mark shows at once in the cached marks of
// q, and the returned Op sends it; when GitHub refuses it, the marks are rolled
// back. The mark is the viewer's own, so read access is enough. The marks that
// are not cached are not read for it: the send still happens.
func (s *Service) SetViewed(q ViewedQuery, path string, viewed bool) *optimistic.Op {
	what := "unmark"
	state := core.Unviewed
	if viewed {
		what, state = "mark", core.Viewed
	}
	apply := func() func() {
		undo, _ := s.pullViewed.Mutate(q.key(), func(v core.PullViewed) core.PullViewed {
			// The map is shared with whoever holds the cached value.
			states := maps.Clone(v.States)
			if states == nil {
				states = map[string]core.ViewedState{}
			}
			if state == core.Unviewed {
				delete(states, path)
			} else {
				states[path] = state
			}
			v.States = states
			return v
		})
		return undo
	}
	return optimistic.New(func(ctx context.Context) error {
		id := ""
		if e, st := s.details.Get(detailKey(q.Repo, q.Number)); st != cache.Miss {
			id = e.Value.ID
		}
		if id == "" {
			var err error
			if id, err = s.api.PullRequestID(ctx, q.Repo, q.Number); err != nil {
				return fmt.Errorf("%s %s: %w", what, path, err)
			}
		}
		if err := s.api.SetFileViewed(ctx, id, path, viewed); err != nil {
			return err
		}
		// The answer holds nothing to reconcile with, and a read that began
		// before the change may have replaced the marks since: apply it again.
		apply()
		return nil
	}, apply())
}
