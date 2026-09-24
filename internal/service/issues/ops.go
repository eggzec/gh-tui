package issues

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// Each change is shown in every cached list page and detail that holds the
// issue as soon as it is made. When GitHub confirms it, the cache takes
// GitHub's version, and the repository's lists are marked stale because the
// change may move the issue within them or out of them.
//
// Changes are never kept in the store before GitHub confirms them, so a
// rollback leaves nothing behind there. The store gets what GitHub sends
// back, or the pages read again after the change.

// Close closes an issue.
func (s *Service) Close(repo core.RepoRef, number int) *optimistic.Op {
	return s.setState(repo, number, core.StateClosed, "close")
}

// Reopen reopens a closed issue.
func (s *Service) Reopen(repo core.RepoRef, number int) *optimistic.Op {
	return s.setState(repo, number, core.StateOpen, "reopen")
}

func (s *Service) setState(repo core.RepoRef, number int, state core.State, verb string) *optimistic.Op {
	rollbacks := s.update(repo, number, func(it core.Issue) core.Issue {
		it.State = state
		return it
	})
	return optimistic.New(func(ctx context.Context) error {
		got, err := s.api.SetIssueState(ctx, repo, number, state)
		if err != nil {
			return fmt.Errorf("%s issue %s#%d: %w", verb, repo, number, err)
		}
		s.update(repo, number, func(core.Issue) core.Issue { return got })
		// GitHub sent the whole issue, so it is kept for the next session.
		// It has no validators, which belong to what was read before.
		key := issueKey(repo, number)
		_ = s.keptIssues.Save(key, cache.Entry[core.Issue]{Value: got, Tags: []string{repoTag(repo), key}})
		s.lists.InvalidateTag(repoTag(repo))
		return nil
	}, rollbacks...)
}

// AddLabels adds labels to an issue by name. Until GitHub answers, a label
// that no cached issue of the repository has is shown with its name only.
func (s *Service) AddLabels(repo core.RepoRef, number int, names []string) *optimistic.Op {
	names = slices.Clone(names)
	known := s.knownLabels(repo)
	rollbacks := s.update(repo, number, func(it core.Issue) core.Issue {
		labels := slices.Clone(it.Labels)
		for _, name := range names {
			if slices.ContainsFunc(labels, labelNamed(name)) {
				continue
			}
			l, ok := known[strings.ToLower(name)]
			if !ok {
				l = core.Label{Name: name}
			}
			labels = append(labels, l)
		}
		it.Labels = labels
		return it
	})
	return optimistic.New(func(ctx context.Context) error {
		labels, err := s.api.AddIssueLabels(ctx, repo, number, names)
		if err != nil {
			return fmt.Errorf("label issue %s#%d: %w", repo, number, err)
		}
		s.setLabels(repo, number, labels)
		return nil
	}, rollbacks...)
}

// RemoveLabel removes a label from an issue.
func (s *Service) RemoveLabel(repo core.RepoRef, number int, name string) *optimistic.Op {
	rollbacks := s.update(repo, number, func(it core.Issue) core.Issue {
		it.Labels = slices.DeleteFunc(slices.Clone(it.Labels), labelNamed(name))
		return it
	})
	return optimistic.New(func(ctx context.Context) error {
		labels, err := s.api.RemoveIssueLabel(ctx, repo, number, name)
		if err != nil {
			return fmt.Errorf("unlabel issue %s#%d: %w", repo, number, err)
		}
		s.setLabels(repo, number, labels)
		return nil
	}, rollbacks...)
}

func (s *Service) setLabels(repo core.RepoRef, number int, labels []core.Label) {
	s.update(repo, number, func(it core.Issue) core.Issue {
		it.Labels = labels
		return it
	})
	s.lists.InvalidateTag(repoTag(repo))
}

// GitHub treats label names that differ only in case as the same label.
func labelNamed(name string) func(core.Label) bool {
	return func(l core.Label) bool { return strings.EqualFold(l.Name, name) }
}

// knownLabels returns the labels of the repository's cached issues by
// lower-case name.
func (s *Service) knownLabels(repo core.RepoRef) map[string]core.Label {
	known := make(map[string]core.Label)
	add := func(labels []core.Label) {
		for _, l := range labels {
			known[strings.ToLower(l.Name)] = l
		}
	}
	for _, p := range s.lists.Tagged(repoTag(repo)) {
		for i := range p.Items {
			add(p.Items[i].Labels)
		}
	}
	issues := s.issues.Tagged(repoTag(repo))
	for i := range issues {
		add(issues[i].Labels)
	}
	return known
}

// pendingPrefix starts the ID of a comment that GitHub hasn't confirmed.
const pendingPrefix = "pending:"

// IsPending reports whether c is a comment that is shown before GitHub
// confirmed it.
func IsPending(c core.Comment) bool {
	return strings.HasPrefix(c.ID, pendingPrefix)
}

// Comment adds a comment to an issue. Until GitHub answers, the comment has
// a temporary ID, for which IsPending reports true, and the viewer set with
// WithViewer as its author. It is shown only on the cached last pages of the
// issue's comments; otherwise only the issue's comment count changes, and
// the comment appears when the tui pages to the end.
func (s *Service) Comment(repo core.RepoRef, number int, body string) *optimistic.Op {
	now := time.Now()
	pending := core.Comment{
		ID:        pendingPrefix + strconv.FormatUint(s.pending.Add(1), 10),
		Author:    core.User{Login: s.viewer},
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
	key := issueKey(repo, number)
	rollbacks := s.update(repo, number, func(it core.Issue) core.Issue {
		it.Comments++
		return it
	})
	rollbacks = append(rollbacks, s.comments.MutateTag(key, pages(func(p core.Page[core.Comment]) (core.Page[core.Comment], bool) {
		if !p.Last() {
			return p, false
		}
		p.Items = append(slices.Clip(p.Items), pending)
		return p, true
	})))

	return optimistic.New(func(ctx context.Context) error {
		got, err := s.api.CreateIssueComment(ctx, repo, number, body)
		if err != nil {
			return fmt.Errorf("comment on issue %s#%d: %w", repo, number, err)
		}
		s.comments.MutateTag(key, pages(func(p core.Page[core.Comment]) (core.Page[core.Comment], bool) {
			// A refetch may have dropped the pending comment, or already
			// brought the real one, or ended the page before it.
			if i := slices.IndexFunc(p.Items, func(c core.Comment) bool { return c.ID == pending.ID }); i >= 0 {
				p.Items = slices.Clone(p.Items)
				p.Items[i] = got
				return p, true
			}
			if !p.Last() || slices.ContainsFunc(p.Items, func(c core.Comment) bool { return c.ID == got.ID }) {
				return p, false
			}
			p.Items = append(slices.Clip(p.Items), got)
			return p, true
		}))
		s.lists.InvalidateTag(repoTag(repo))
		return nil
	}, rollbacks...)
}

// update applies fn to the issue in every cached list page and in its
// cached detail, and returns the rollbacks. fn must not modify the slices of
// its argument.
func (s *Service) update(repo core.RepoRef, number int, fn func(core.Issue) core.Issue) []func() {
	key := issueKey(repo, number)
	// The change moves the issue past the version the list showed, so
	// what is cached of it is only as good as its TTL.
	s.seen.Delete(key)
	inList := s.lists.MutateTag(key, func(p core.Page[core.Issue]) (core.Page[core.Issue], bool) {
		i := slices.IndexFunc(p.Items, func(it core.Issue) bool { return it.Number == number })
		if i < 0 {
			return p, false
		}
		p.Items = slices.Clone(p.Items)
		p.Items[i] = fn(p.Items[i])
		return p, true
	})
	inDetail, _ := s.issues.Mutate(key, fn)
	return []func(){inList, inDetail}
}

// pages applies fn to the page of a stamped comment page, for MutateTag.
func pages(fn func(core.Page[core.Comment]) (core.Page[core.Comment], bool)) func(stampedComments) (stampedComments, bool) {
	return func(p stampedComments) (stampedComments, bool) {
		var changed bool
		p.Value, changed = fn(p.Value)
		return p, changed
	}
}
