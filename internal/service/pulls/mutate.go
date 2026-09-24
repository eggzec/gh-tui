package pulls

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// A change is shown at once in every cached list page and detail that holds
// the pull request. A page whose state filter the pull request no longer
// matches, such as the open list after a merge, keeps showing it with its
// new state until the page is fetched again. Once the server confirms the
// change, every list page of the repository is marked stale for that reason,
// and because the change reorders pull requests by update time.

// Merge merges pull request number of repo with method.
func (s *Service) Merge(repo core.RepoRef, number int, method core.MergeMethod) *optimistic.Op {
	now := s.now()
	return s.change("merge", repo, number, func(pr *core.PullRequest) {
		pr.State, pr.MergedAt = core.StateMerged, now
	}, func(ctx context.Context, id string) (core.PullRequest, error) {
		return s.api.MergePullRequest(ctx, id, method)
	})
}

// Close closes pull request number of repo without merging it.
func (s *Service) Close(repo core.RepoRef, number int) *optimistic.Op {
	return s.change("close", repo, number, func(pr *core.PullRequest) {
		pr.State = core.StateClosed
	}, s.api.ClosePullRequest)
}

// Reopen reopens the closed pull request number of repo.
func (s *Service) Reopen(repo core.RepoRef, number int) *optimistic.Op {
	return s.change("reopen", repo, number, func(pr *core.PullRequest) {
		pr.State = core.StateOpen
	}, s.api.ReopenPullRequest)
}

// MarkReady marks the draft pull request number of repo as ready for review.
func (s *Service) MarkReady(repo core.RepoRef, number int) *optimistic.Op {
	return s.change("mark ready", repo, number, func(pr *core.PullRequest) {
		pr.Draft = false
	}, s.api.MarkPullRequestReady)
}

// ConvertToDraft turns pull request number of repo back into a draft.
func (s *Service) ConvertToDraft(repo core.RepoRef, number int) *optimistic.Op {
	return s.change("convert to draft", repo, number, func(pr *core.PullRequest) {
		pr.Draft = true
	}, s.api.ConvertPullRequestToDraft)
}

// change applies edit to every cached copy of pull request number of repo
// and returns the Op that sends the change with send. What names the change
// in errors.
func (s *Service) change(
	what string, repo core.RepoRef, number int,
	edit func(*core.PullRequest),
	send func(ctx context.Context, id string) (core.PullRequest, error),
) *optimistic.Op {
	// The change moves the pull request past the version the list showed,
	// so what is cached of it is only as good as its TTL.
	s.seen.Delete(detailKey(repo, number))
	// The mutations need the node ID, which any cached copy has.
	var id string
	undoLists := s.lists.MutateTag(repoTag(repo), func(p core.Page[core.PullRequest]) (core.Page[core.PullRequest], bool) {
		return replace(p, number, func(pr core.PullRequest) core.PullRequest {
			id = cmp.Or(id, pr.ID)
			edit(&pr)
			return pr
		})
	})
	undoDetail, _ := s.details.Mutate(detailKey(repo, number), func(d core.PullRequestDetail) core.PullRequestDetail {
		id = cmp.Or(id, d.ID)
		edit(&d.PullRequest)
		return d
	})

	return optimistic.New(func(ctx context.Context) error {
		nodeID := id
		if nodeID == "" {
			var err error
			if nodeID, err = s.api.PullRequestID(ctx, repo, number); err != nil {
				return fmt.Errorf("%s pull %s#%d: %w", what, repo, number, err)
			}
		}
		pr, err := send(ctx, nodeID)
		if err != nil {
			return fmt.Errorf("%s pull %s#%d: %w", what, repo, number, err)
		}
		s.reconcile(repo, number, pr)
		return nil
	}, undoLists, undoDetail)
}

// reconcile stores pr, as the server returned it, in place of every cached
// copy of pull request number of repo.
func (s *Service) reconcile(repo core.RepoRef, number int, pr core.PullRequest) {
	tag := repoTag(repo)
	s.lists.MutateTag(tag, func(p core.Page[core.PullRequest]) (core.Page[core.PullRequest], bool) {
		return replace(p, number, func(core.PullRequest) core.PullRequest { return pr })
	})
	key := detailKey(repo, number)
	s.details.Mutate(key, func(d core.PullRequestDetail) core.PullRequestDetail {
		// Mutations don't return the body, and they don't change it.
		body := d.Body
		d.PullRequest = pr
		d.Body = body
		return d
	})
	// What GitHub confirmed is kept for the next session. Until then only
	// memory has the change, so a rollback leaves nothing behind.
	if e, st := s.details.Get(key); st != cache.Miss {
		_ = s.keptDetails.Save(key, cache.Entry[core.PullRequestDetail]{Value: e.Value, Tags: e.Tags})
	}
	s.lists.InvalidateTag(tag)
}

// replace returns p with pull request number replaced by f of it, and
// whether p holds it. It copies the items rather than change the cached
// ones.
func replace(p core.Page[core.PullRequest], number int, f func(core.PullRequest) core.PullRequest) (core.Page[core.PullRequest], bool) {
	i := slices.IndexFunc(p.Items, func(pr core.PullRequest) bool { return pr.Number == number })
	if i < 0 {
		return p, false
	}
	p.Items = slices.Clone(p.Items)
	p.Items[i] = f(p.Items[i])
	return p, true
}
