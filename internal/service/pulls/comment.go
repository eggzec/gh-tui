package pulls

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// pendingPrefix starts the ID of a comment that GitHub hasn't confirmed.
const pendingPrefix = "pending:"

// IsPending reports whether c is a comment that is shown before GitHub
// confirmed it.
func IsPending(c core.Comment) bool {
	return Unconfirmed(c.ID)
}

// Comment adds a comment to the conversation of pull request number. Until
// GitHub answers, the comment has a temporary ID, for which IsPending reports
// true, and author as its login. It is shown only on the cached last page of
// the comments; otherwise it appears when the tui pages to the end. If GitHub
// refuses it, the comment is taken out again.
func (s *Service) Comment(repo core.RepoRef, number int, body, author string) *optimistic.Op {
	if err := s.refused(repo); err != nil {
		return optimistic.Refused(fmt.Errorf("comment on pull %s#%d: %w", repo, number, err))
	}
	now := time.Now()
	pending := core.Comment{
		ID:        pendingPrefix + strconv.FormatUint(s.pending.Add(1), 10),
		Author:    core.User{Login: author},
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
	key := detailKey(repo, number)
	undo := s.comments.MutateTag(key, pages(func(p core.Page[core.Comment]) (core.Page[core.Comment], bool) {
		if !p.Last() {
			return p, false
		}
		p.Items = append(slices.Clip(p.Items), pending)
		return p, true
	}))
	return optimistic.New(func(ctx context.Context) error {
		got, err := s.api.CreateIssueComment(ctx, repo, number, body)
		if err != nil {
			return fmt.Errorf("comment on pull %s#%d: %w", repo, number, err)
		}
		// The comment shows in the timeline, whose cached pages lack it.
		forget(s.timelines, s.keptTimelines, key)
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
		return nil
	}, undo)
}

// pages applies fn to the page of a stamped comment page, for MutateTag.
func pages(fn func(core.Page[core.Comment]) (core.Page[core.Comment], bool)) func(stampedComments) (stampedComments, bool) {
	return func(p stampedComments) (stampedComments, bool) {
		var changed bool
		p.Value, changed = fn(p.Value)
		return p, changed
	}
}
