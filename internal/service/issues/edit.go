package issues

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/edit"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// errCannotEdit is why an issue that the viewer may not update is not edited.
var errCannotEdit = errors.New("you may not edit this issue")

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

// forgetTimeline marks stale the cached timeline pages of the issue with key,
// and drops them from the store, since they lack what GitHub just accepted.
func (s *Service) forgetTimeline(key string) {
	edit.Unkeep(s.timelines, s.keptTimelines, key)
	s.timelines.InvalidateTag(key)
}

// staleIssue marks stale what is cached of issue number of repo, and of the
// repository's lists, which show it, when its state at GitHub is no longer
// what the cache holds or can tell.
func (s *Service) staleIssue(repo core.RepoRef, number int) {
	key := issueKey(repo, number)
	s.seen.Delete(key)
	s.issues.Invalidate(key)
	s.keptIssues.Delete(key)
	s.lists.InvalidateTag(repoTag(repo))
}

// EditIssue sets the title and the description of issue number of repo. They
// show at once in the cached list rows and detail, the description with the
// time it was edited, and the returned Op sends them. When GitHub accepts
// them, the issue's cached copies take what it returns and the repository's
// lists are marked stale. A token that may not change the repository is
// refused, and so is a cached detail that says the viewer may not update the
// issue, and nothing changes then. Without a cached detail, GitHub's refusal
// is what rolls the change back.
func (s *Service) EditIssue(repo core.RepoRef, number int, title, body string) *optimistic.Op {
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("edit issue %s#%d: %w", repo, number, err))
	}
	key := issueKey(repo, number)
	if e, st := s.issues.Get(key); st != cache.Miss && e.Value.Caps.Known && !e.Value.Caps.Update {
		return optimistic.Refused(fmt.Errorf("edit issue %s#%d: %w", repo, number, errCannotEdit))
	}
	at := s.now()
	rollbacks := s.update(repo, number, func(it core.Issue) core.Issue {
		it.Title, it.Body, it.BodyEditedAt = title, body, at
		return it
	})
	return optimistic.New(func(ctx context.Context) error {
		got, err := s.api.UpdateIssueText(ctx, repo, number, title, body)
		if err != nil {
			return fmt.Errorf("edit issue %s#%d: %w", repo, number, err)
		}
		edited := edit.At(core.Edited{At: got.UpdatedAt}, s.now())
		s.update(repo, number, func(it core.Issue) core.Issue {
			it.Title, it.Body, it.BodyEditedAt, it.UpdatedAt = got.Title, got.Body, edited, got.UpdatedAt
			return it
		})
		if e, st := s.issues.Get(key); st != cache.Miss {
			// What is kept has no validators, which belong to a read.
			_ = s.keptIssues.Save(key, cache.Entry[core.Issue]{Value: e.Value, Tags: e.Tags})
		}
		s.forgetTimeline(key)
		s.lists.InvalidateTag(repoTag(repo))
		return nil
	}, rollbacks...)
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
	undo := set(body, s.now())
	return optimistic.New(func(ctx context.Context) error {
		got, err := s.api.EditIssueComment(ctx, id, body)
		if err != nil {
			return fmt.Errorf("edit comment on issue %s#%d: %w", repo, number, err)
		}
		set(got.Body, edit.At(got, s.now()))
		edit.Unkeep(s.comments, s.keptComments, key)
		edit.Unkeep(s.timelines, s.keptTimelines, key)
		return nil
	}, undo)
}

// React gives the viewer's reaction content to comment id of issue number of
// repo in the cached comments and timeline at once, or takes it back when add
// is false. The returned Op sends it. GitHub's answer holds nothing to store,
// so the kept copies are dropped, which lack the change.
func (s *Service) React(repo core.RepoRef, number int, id, content string, add bool) *optimistic.Op {
	what := "react"
	if err := s.refusedComment(repo, what, number, id); err != nil {
		return optimistic.Refused(err)
	}
	key := issueKey(repo, number)
	undoComments := s.comments.MutateTag(key, pages(func(p core.Page[core.Comment]) (core.Page[core.Comment], bool) {
		items, ok := edit.Patch(p.Items, func(c core.Comment) bool { return c.ID == id }, func(c *core.Comment) {
			c.Reactions = edit.Reacted(c.Reactions, content, add)
		})
		p.Items = items
		return p, ok
	}))
	undoTimeline := s.timelines.MutateTag(key, edit.ReactOnTimeline(id, content, add))
	return optimistic.New(func(ctx context.Context) error {
		send := s.api.AddReaction
		if !add {
			send = s.api.RemoveReaction
		}
		if err := send(ctx, id, content); err != nil {
			return fmt.Errorf("react on comment of issue %s#%d: %w", repo, number, err)
		}
		edit.Unkeep(s.comments, s.keptComments, key)
		edit.Unkeep(s.timelines, s.keptTimelines, key)
		return nil
	}, func() { undoTimeline(); undoComments() })
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
		s.staleIssue(repo, number)
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
	// A change that failed after some call went through is on GitHub, so the
	// rollback must not leave the old view as if it were current. The mark
	// goes first in the list, to run after the rollbacks, which skip an entry
	// that is already stale.
	var partial bool
	rollbacks = slices.Insert(rollbacks, 0, func() { s.staleAfterPartial(repo, number, partial) })
	return optimistic.New(func(ctx context.Context) error {
		labels, known, err := s.sendLabels(ctx, repo, number, add, remove)
		if err != nil {
			partial = known
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

// staleAfterPartial marks the issue and its timeline stale if partial, which
// is when a change failed after one of its calls had gone through.
func (s *Service) staleAfterPartial(repo core.RepoRef, number int, partial bool) {
	if !partial {
		return
	}
	s.staleIssue(repo, number)
	s.forgetTimeline(issueKey(repo, number))
}

// sendLabels adds the labels in one call and removes each of the others, which
// GitHub has no call for in bulk. A label that is already gone is no failure.
// known is false when no call answered with the labels, and on an error it
// tells whether one went through before it.
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
			return nil, known, err
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
	// As for the labels, a failure after the users were added is stale.
	var partial bool
	rollbacks = slices.Insert(rollbacks, 0, func() { s.staleAfterPartial(repo, number, partial) })
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
				partial = len(add) > 0
				return fmt.Errorf("assign issue %s#%d: %w", repo, number, err)
			}
		}
		s.update(repo, number, func(it core.Issue) core.Issue {
			// The answer is the whole issue, so its labels, which an
			// assignment doesn't change, are as current as its assignees.
			it.Assignees, it.Labels = got.Assignees, got.Labels
			return it
		})
		s.forgetTimeline(issueKey(repo, number))
		s.lists.InvalidateTag(repoTag(repo))
		return nil
	}, rollbacks...)
}
