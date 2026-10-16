package pulls

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// labelsAPI is what changes the labels and assignees of a pull request. They
// are those of its issue, which REST changes by number, so no node ID is
// needed. The label calls answer with the labels left, the assignee calls
// with the issue as GitHub left it.
type labelsAPI interface {
	AddIssueLabels(ctx context.Context, repo core.RepoRef, number int, names []string) ([]core.Label, error)
	RemoveIssueLabel(ctx context.Context, repo core.RepoRef, number int, name string) ([]core.Label, error)
	AddIssueAssignees(ctx context.Context, repo core.RepoRef, number int, logins []string) (core.Issue, error)
	RemoveIssueAssignees(ctx context.Context, repo core.RepoRef, number int, logins []string) (core.Issue, error)
}

// EditLabels adds the labels add to pull request number of repo and takes the
// labels named in remove off it, and shows it at once on every cached copy.
// The Op sends it, and GitHub's answer replaces what was shown. add carries
// whole labels so that the chip has its color before GitHub answers; GitHub
// creates a label it doesn't know, so the caller offers existing ones.
func (s *Service) EditLabels(repo core.RepoRef, number int, add []core.Label, remove []string) *optimistic.Op {
	if len(add) == 0 && len(remove) == 0 {
		return optimistic.New(func(context.Context) error { return nil })
	}
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("label pull %s#%d: %w", repo, number, err))
	}
	undos := s.editPull(repo, number, func(pr *core.PullRequest) {
		pr.Labels = edited(pr.Labels, add, remove, func(l core.Label) string { return l.Name })
	})
	return optimistic.New(func(ctx context.Context) error {
		labels, known, err := s.sendLabels(ctx, repo, number, add, remove)
		if err != nil {
			s.staleEdit(repo, number, undos)
			return fmt.Errorf("label pull %s#%d: %w", repo, number, err)
		}
		if known {
			s.editPull(repo, number, func(pr *core.PullRequest) { pr.Labels = labels })
		}
		s.confirmEdit(repo, number)
		return nil
	}, undos...)
}

// sendLabels adds, then removes one label at a time, and returns the labels
// GitHub answered last, and whether it answered with any.
func (s *Service) sendLabels(ctx context.Context, repo core.RepoRef, number int, add []core.Label, remove []string) ([]core.Label, bool, error) {
	var labels []core.Label
	var known bool
	if len(add) > 0 {
		names := make([]string, len(add))
		for i, l := range add {
			names[i] = l.Name
		}
		got, err := s.api.AddIssueLabels(ctx, repo, number, names)
		if err != nil {
			return nil, false, err
		}
		labels, known = got, true
	}
	for _, name := range remove {
		got, err := s.api.RemoveIssueLabel(ctx, repo, number, name)
		if errors.Is(err, core.ErrNotFound) {
			// It is gone already.
			continue
		}
		if err != nil {
			return nil, false, err
		}
		labels, known = got, true
	}
	return labels, known, nil
}

// EditAssignees assigns the users add to pull request number of repo and
// takes the logins in remove off it, and shows it at once on every cached
// copy. GitHub skips a user it may not assign, so the issue it answers with
// replaces what was shown.
func (s *Service) EditAssignees(repo core.RepoRef, number int, add []core.User, remove []string) *optimistic.Op {
	if len(add) == 0 && len(remove) == 0 {
		return optimistic.New(func(context.Context) error { return nil })
	}
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("assign pull %s#%d: %w", repo, number, err))
	}
	undos := s.editPull(repo, number, func(pr *core.PullRequest) {
		pr.Assignees = edited(pr.Assignees, add, remove, func(u core.User) string { return u.Login })
	})
	return optimistic.New(func(ctx context.Context) error {
		var it core.Issue
		var err error
		if len(add) > 0 {
			logins := make([]string, len(add))
			for i, u := range add {
				logins[i] = u.Login
			}
			if it, err = s.api.AddIssueAssignees(ctx, repo, number, logins); err != nil {
				s.staleEdit(repo, number, undos)
				return fmt.Errorf("assign pull %s#%d: %w", repo, number, err)
			}
		}
		if len(remove) > 0 {
			if it, err = s.api.RemoveIssueAssignees(ctx, repo, number, remove); err != nil {
				s.staleEdit(repo, number, undos)
				return fmt.Errorf("assign pull %s#%d: %w", repo, number, err)
			}
		}
		s.editPull(repo, number, func(pr *core.PullRequest) {
			pr.Assignees, pr.Labels = it.Assignees, it.Labels
		})
		s.confirmEdit(repo, number)
		return nil
	}, undos...)
}

// editPull applies edit to every cached copy of pull request number of repo,
// without touching what the copies share, and returns the rollbacks.
func (s *Service) editPull(repo core.RepoRef, number int, edit func(*core.PullRequest)) []func() {
	key := detailKey(repo, number)
	// The change moves the pull request past the version the list showed.
	s.seen.Delete(key)
	undoLists := s.lists.MutateTag(repoTag(repo), func(p listPage) (listPage, bool) {
		return replace(p, number, func(pr core.PullRequest) core.PullRequest {
			edit(&pr)
			return pr
		})
	})
	undoDetail, _ := s.details.Mutate(key, func(d core.PullRequestDetail) core.PullRequestDetail {
		edit(&d.PullRequest)
		return d
	})
	return []func(){undoLists, undoDetail}
}

// confirmEdit settles what GitHub accepted: the timeline has an event now, the
// detail is kept for the next session, and the lists are read again.
func (s *Service) confirmEdit(repo core.RepoRef, number int) {
	key := detailKey(repo, number)
	forget(s.timelines, s.keptTimelines, key)
	if e, st := s.details.Get(key); st != cache.Miss {
		_ = s.keptDetails.Save(key, cache.Entry[core.PullRequestDetail]{Value: e.Value, Tags: e.Tags})
	}
	s.lists.InvalidateTag(repoTag(repo))
}

// staleEdit rolls the copies back with undos and marks them stale, after a
// failed edit. One of its requests may have landed before another failed, so
// the rollback alone can't say what GitHub has. It rolls back first, since a
// copy written since is only marked stale by a rollback and not restored.
func (s *Service) staleEdit(repo core.RepoRef, number int, undos []func()) {
	for _, undo := range slices.Backward(undos) {
		undo()
	}
	s.details.Invalidate(detailKey(repo, number))
	s.lists.InvalidateTag(repoTag(repo))
}

// edited returns items without those whose name is in remove, which may differ
// in case, and with those of add that aren't there yet. It is a new slice,
// since the cache and the models share items.
func edited[T any](items, add []T, remove []string, name func(T) string) []T {
	out := make([]T, 0, len(items)+len(add))
	for _, it := range items {
		if !slices.ContainsFunc(remove, func(n string) bool { return strings.EqualFold(n, name(it)) }) {
			out = append(out, it)
		}
	}
	for _, a := range add {
		if !slices.ContainsFunc(out, func(o T) bool { return strings.EqualFold(name(o), name(a)) }) {
			out = append(out, a)
		}
	}
	return out
}
