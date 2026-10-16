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

// Merge merges pull request number of repo with method, with the commit
// message msg. A head other than "" pins the merge to that commit, the head
// the user saw, so that GitHub refuses it if new commits were pushed since.
func (s *Service) Merge(repo core.RepoRef, number int, method core.MergeMethod, msg core.MergeMessage, head string) *optimistic.Op {
	now := s.now()
	return s.change("merge", repo, number, func(pr *core.PullRequest) {
		pr.State, pr.MergedAt = core.StateMerged, now
	}, func(ctx context.Context, id string) (core.PullRequest, error) {
		return s.api.MergePullRequest(ctx, id, method, msg, head)
	})
}

// AutoMerge turns auto-merge on for pull request number of repo: GitHub
// merges it with method and the commit message msg once its checks pass and
// its reviews are in. A head other than "" pins it to that commit.
func (s *Service) AutoMerge(repo core.RepoRef, number int, method core.MergeMethod, msg core.MergeMessage, head string) *optimistic.Op {
	return s.changeMerge("enable auto-merge on", repo, number, func(m *core.MergeInfo) {
		m.AutoMerge = &core.AutoMerge{Method: method}
		// The viewer who turned it on may turn it off, and not on again.
		m.CanDisableAutoMerge, m.CanAutoMerge = true, false
	}, func(ctx context.Context, id string) (core.PullRequest, error) {
		return s.api.AutoMergePullRequest(ctx, id, method, msg, head)
	})
}

// StopAutoMerge turns auto-merge off for pull request number of repo.
func (s *Service) StopAutoMerge(repo core.RepoRef, number int) *optimistic.Op {
	return s.changeMerge("disable auto-merge on", repo, number, func(m *core.MergeInfo) {
		m.AutoMerge = nil
		m.CanDisableAutoMerge, m.CanAutoMerge = false, true
	}, s.api.StopAutoMergePullRequest)
}

// Enqueue adds pull request number of repo to the merge queue of its base
// branch. A head other than "" pins it to that commit.
func (s *Service) Enqueue(repo core.RepoRef, number int, head string) *optimistic.Op {
	return s.changeMerge("enqueue", repo, number, func(m *core.MergeInfo) {
		m.Queue.Queued = true
	}, func(ctx context.Context, id string) (core.PullRequest, error) {
		return s.api.EnqueuePullRequest(ctx, id, head)
	})
}

// changeMerge is change for what the detail holds of merging: the pull
// request itself stays as it is until GitHub says otherwise.
func (s *Service) changeMerge(
	what string, repo core.RepoRef, number int,
	edit func(*core.MergeInfo),
	send func(ctx context.Context, id string) (core.PullRequest, error),
) *optimistic.Op {
	return s.changeDetail(what, repo, number, func(*core.PullRequest) {}, edit, send)
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

// UpdateBranch merges the base branch of pull request number of repo into
// its head branch. A head other than "" pins the update to that commit, the
// head the user saw, so that GitHub refuses it if new commits were pushed
// since. A refusal, for a branch that is up to date or one that conflicts
// with its base, is a *core.RefusedError with GitHub's reason.
//
// The update changes no value that can be shown before GitHub does it, and
// GitHub answers before the head moves. So the Op changes nothing at once,
// and once GitHub accepts the update, it marks stale what is cached of the
// pull request: its detail, with the merge state and failing checks that
// move with the head, its timeline, files and commits, and the list pages.
//
// GitHub answers before the merge lands, so a detail read right after the
// update succeeds may still show the old head, and is then cached as fresh
// with it. The caller reads again after a short delay, or relies on the poll
// to find the new head.
//
// The checks of the actions service move with the head too, and are that
// service's to mark: the caller runs actions.Service.Invalidate with the
// repository, since that service keys its checks by commit and has no way
// to find those of one pull request.
func (s *Service) UpdateBranch(repo core.RepoRef, number int, head string) *optimistic.Op {
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("update branch of pull %s#%d: %w", repo, number, err))
	}
	return optimistic.New(func(ctx context.Context) error {
		if err := s.api.UpdatePullRequestBranch(ctx, repo, number, head); err != nil {
			return fmt.Errorf("update branch of pull %s#%d: %w", repo, number, err)
		}
		s.stale(repo, number)
		return nil
	})
}

// stale marks stale everything cached of pull request number of repo that
// its head decides, for a change GitHub accepted but did not return.
func (s *Service) stale(repo core.RepoRef, number int) {
	key := detailKey(repo, number)
	s.seen.Delete(key)
	forget(s.timelines, s.keptTimelines, key)
	s.details.Invalidate(key)
	// The kept detail is of the old head, and a read that finds no detail in
	// memory must not be served it.
	s.keptDetails.Delete(key)
	s.files.InvalidateTag(key)
	s.commits.InvalidateTag(key)
	s.lists.InvalidateTag(repoTag(repo))
}

// change applies edit to every cached copy of pull request number of repo
// and returns the Op that sends the change with send. What names the change
// in errors. A change the token may not make changes nothing, and the Op
// returns why.
func (s *Service) change(
	what string, repo core.RepoRef, number int,
	edit func(*core.PullRequest),
	send func(ctx context.Context, id string) (core.PullRequest, error),
) *optimistic.Op {
	return s.changeDetail(what, repo, number, edit, nil, send)
}

// changeDetail is change that also applies editMerge, if set, to the merge
// state of the cached detail.
func (s *Service) changeDetail(
	what string, repo core.RepoRef, number int,
	edit func(*core.PullRequest), editMerge func(*core.MergeInfo),
	send func(ctx context.Context, id string) (core.PullRequest, error),
) *optimistic.Op {
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("%s pull %s#%d: %w", what, repo, number, err))
	}
	// The change moves the pull request past the version the list showed,
	// so what is cached of it is only as good as its TTL.
	s.seen.Delete(detailKey(repo, number))
	// The mutations need the node ID, which any cached copy has.
	var id string
	undoLists := s.lists.MutateTag(repoTag(repo), func(p listPage) (listPage, bool) {
		return replace(p, number, func(pr core.PullRequest) core.PullRequest {
			id = cmp.Or(id, pr.ID)
			edit(&pr)
			return pr
		})
	})
	undoDetail, _ := s.details.Mutate(detailKey(repo, number), func(d core.PullRequestDetail) core.PullRequestDetail {
		id = cmp.Or(id, d.ID)
		edit(&d.PullRequest)
		if editMerge != nil {
			editMerge(&d.Merge)
		}
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

// refused returns why the token may not change a pull request of repo, or
// nil when it may, or when that isn't known.
func (s *Service) refused(repo core.RepoRef) error {
	if s.access == nil {
		return nil
	}
	var caps core.RepoCaps
	if s.repos != nil {
		r, _ := s.repos.CachedGet(repo)
		caps = r.Caps
	}
	return s.access.Check(core.NeedWrite(caps))
}

// reconcile stores pr, as the server returned it, in place of every cached
// copy of pull request number of repo.
func (s *Service) reconcile(repo core.RepoRef, number int, pr core.PullRequest) {
	tag := repoTag(repo)
	s.lists.MutateTag(tag, func(p listPage) (listPage, bool) {
		return replace(p, number, func(core.PullRequest) core.PullRequest { return pr })
	})
	key := detailKey(repo, number)
	// The change shows in the timeline as an event, which its pages lack.
	forget(s.timelines, s.keptTimelines, key)
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
func replace(p listPage, number int, f func(core.PullRequest) core.PullRequest) (listPage, bool) {
	items := p.Page.Items
	i := slices.IndexFunc(items, func(pr core.PullRequest) bool { return pr.Number == number })
	if i < 0 {
		return p, false
	}
	p.Page.Items = slices.Clone(items)
	p.Page.Items[i] = f(items[i])
	return p, true
}
