package issues

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/edit"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// errUnsent is why a comment that GitHub hasn't confirmed is not edited or
// deleted: it has no ID to send yet.
var errUnsent = errors.New("the comment isn't confirmed yet")

// Unconfirmed reports whether id is that of a comment that is shown before
// GitHub confirmed it, which can be neither edited nor deleted until it is.
func Unconfirmed(id string) bool {
	return strings.HasPrefix(id, pendingPrefix)
}

// refusedComment returns why comment id of issue number may not be changed,
// or nil. what is the verb of the change, for the error.
func (s *Service) refusedComment(repo core.RepoRef, what string, number int, id string) error {
	err := s.refused(repo)
	if err == nil && Unconfirmed(id) {
		err = errUnsent
	}
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s comment on issue %s#%d: %w", what, repo, number, err)
}

// forgetTimeline marks stale the cached timeline pages of issue number, and
// drops them from the store, since they lack what GitHub just accepted.
func (s *Service) forgetTimeline(key string) {
	edit.Unkeep(s.timelines, s.keptTimelines, key)
	s.timelines.InvalidateTag(key)
}

// EditComment sets the body of comment id of issue number of repo in the
// cached comments and timeline at once. The returned Op sends it, and when
// GitHub accepts it, the body it returns is stored with the time it was
// edited.
func (s *Service) EditComment(repo core.RepoRef, number int, id, body string) *optimistic.Op {
	if err := s.refusedComment(repo, "edit", number, id); err != nil {
		return optimistic.Refused(err)
	}
	key := issueKey(repo, number)
	set := func(body string, at time.Time) func() {
		undoComments := s.comments.MutateTag(key, pages(func(p core.Page[core.Comment]) (core.Page[core.Comment], bool) {
			items, ok := edit.Patch(p.Items, func(c core.Comment) bool { return c.ID == id }, func(c *core.Comment) {
				c.Body, c.UpdatedAt = body, at
			})
			p.Items = items
			return p, ok
		}))
		undoTimeline := s.timelines.MutateTag(key, edit.OnTimeline(id, body, at))
		return func() { undoTimeline(); undoComments() }
	}
	undo := set(body, time.Now())
	return optimistic.New(func(ctx context.Context) error {
		got, err := s.api.EditIssueComment(ctx, id, body)
		if err != nil {
			return fmt.Errorf("edit comment on issue %s#%d: %w", repo, number, err)
		}
		set(got.Body, edit.At(got, time.Now()))
		edit.Unkeep(s.comments, s.keptComments, key)
		edit.Unkeep(s.timelines, s.keptTimelines, key)
		return nil
	}, undo)
}

// DeleteComment takes comment id out of the cached comments and timeline of
// issue number of repo at once, and counts one comment less. The returned Op
// sends it, and when GitHub accepts it, the lists, which count the comment,
// are marked stale.
func (s *Service) DeleteComment(repo core.RepoRef, number int, id string) *optimistic.Op {
	if err := s.refusedComment(repo, "delete", number, id); err != nil {
		return optimistic.Refused(err)
	}
	key := issueKey(repo, number)
	var removed bool
	undoComments := s.comments.MutateTag(key, pages(func(p core.Page[core.Comment]) (core.Page[core.Comment], bool) {
		i := slices.IndexFunc(p.Items, func(c core.Comment) bool { return c.ID == id })
		if i < 0 {
			return p, false
		}
		removed = true
		p.Items = slices.Delete(slices.Clone(p.Items), i, i+1)
		return p, true
	}))
	undoTimeline := s.timelines.MutateTag(key, edit.DropFromTimeline(id))
	rollbacks := []func(){undoComments, undoTimeline}
	if removed {
		rollbacks = append(rollbacks, s.update(repo, number, func(it core.Issue) core.Issue {
			it.Comments = max(it.Comments-1, 0)
			return it
		})...)
	}
	return optimistic.New(func(ctx context.Context) error {
		if err := s.api.DeleteIssueComment(ctx, id); err != nil {
			return fmt.Errorf("delete comment on issue %s#%d: %w", repo, number, err)
		}
		edit.Unkeep(s.comments, s.keptComments, key)
		edit.Unkeep(s.timelines, s.keptTimelines, key)
		s.seen.Delete(key)
		s.issues.Invalidate(key)
		s.keptIssues.Delete(key)
		s.lists.InvalidateTag(repoTag(repo))
		return nil
	}, rollbacks...)
}

// EditLabels adds the labels add to issue number of repo and takes the labels
// named in remove off it, and shows it at once on every cached copy. The Op
// sends it, and GitHub's answer replaces what was shown. add carries whole
// labels so that the chip has its color before GitHub answers.
func (s *Service) EditLabels(repo core.RepoRef, number int, add []core.Label, remove []string) *optimistic.Op {
	if len(add) == 0 && len(remove) == 0 {
		return optimistic.New(func(context.Context) error { return nil })
	}
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("label issue %s#%d: %w", repo, number, err))
	}
	add, remove = slices.Clone(add), slices.Clone(remove)
	rollbacks := s.update(repo, number, func(it core.Issue) core.Issue {
		it.Labels = edit.Merged(it.Labels, add, remove, func(l core.Label) string { return l.Name })
		return it
	})
	return optimistic.New(func(ctx context.Context) error {
		labels, known, err := s.sendLabels(ctx, repo, number, add, remove)
		if err != nil {
			return fmt.Errorf("label issue %s#%d: %w", repo, number, err)
		}
		if known {
			s.setLabels(repo, number, labels)
		}
		s.forgetTimeline(issueKey(repo, number))
		s.lists.InvalidateTag(repoTag(repo))
		return nil
	}, rollbacks...)
}

// sendLabels adds the labels in one call and removes each of the others, which
// GitHub has no call for in bulk. A label that is already gone is no failure.
// known is false when the changes ended with no answer that lists the labels.
func (s *Service) sendLabels(ctx context.Context, repo core.RepoRef, number int, add []core.Label, remove []string) (labels []core.Label, known bool, err error) {
	if len(add) > 0 {
		names := make([]string, len(add))
		for i, l := range add {
			names[i] = l.Name
		}
		if labels, err = s.api.AddIssueLabels(ctx, repo, number, names); err != nil {
			return nil, false, err
		}
		known = true
	}
	for _, name := range remove {
		got, err := s.api.RemoveIssueLabel(ctx, repo, number, name)
		if errors.Is(err, core.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		labels, known = got, true
	}
	return labels, known, nil
}

// EditAssignees adds the users add to the assignees of issue number of repo
// and takes those whose logins are in remove off it, and shows it at once on
// every cached copy. The Op sends it, and GitHub's answer replaces what was
// shown.
func (s *Service) EditAssignees(repo core.RepoRef, number int, add []core.User, remove []string) *optimistic.Op {
	if len(add) == 0 && len(remove) == 0 {
		return optimistic.New(func(context.Context) error { return nil })
	}
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("assign issue %s#%d: %w", repo, number, err))
	}
	add, remove = slices.Clone(add), slices.Clone(remove)
	rollbacks := s.update(repo, number, func(it core.Issue) core.Issue {
		it.Assignees = edit.Merged(it.Assignees, add, remove, func(u core.User) string { return u.Login })
		return it
	})
	return optimistic.New(func(ctx context.Context) error {
		var got core.Issue
		var err error
		if len(add) > 0 {
			logins := make([]string, len(add))
			for i, u := range add {
				logins[i] = u.Login
			}
			if got, err = s.api.AddIssueAssignees(ctx, repo, number, logins); err != nil {
				return fmt.Errorf("assign issue %s#%d: %w", repo, number, err)
			}
		}
		if len(remove) > 0 {
			if got, err = s.api.RemoveIssueAssignees(ctx, repo, number, remove); err != nil {
				return fmt.Errorf("assign issue %s#%d: %w", repo, number, err)
			}
		}
		s.update(repo, number, func(it core.Issue) core.Issue {
			it.Assignees, it.Labels = got.Assignees, got.Labels
			return it
		})
		s.forgetTimeline(issueKey(repo, number))
		s.lists.InvalidateTag(repoTag(repo))
		return nil
	}, rollbacks...)
}
