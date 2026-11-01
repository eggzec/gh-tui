package issues

import (
	"context"
	"errors"

	"github.com/eggzec/gh-tui/internal/core"
)

// Prototype: the timeline and comment edits are not cached and not optimistic.
// The client is asserted for them so the API interface and its fakes stay as
// they are.

// TimelineQuery names a page of an issue's timeline.
type TimelineQuery struct {
	Repo     core.RepoRef
	Number   int
	Cursor   string
	PageSize int
}

type timelineAPI interface {
	ListIssueTimeline(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.TimelineItem], error)
}

type editAPI interface {
	EditIssueComment(ctx context.Context, id, body string) (core.Edited, error)
}

// Timeline reads a page of the timeline of an issue from GitHub.
func (s *Service) Timeline(ctx context.Context, q TimelineQuery) (core.Page[core.TimelineItem], error) {
	api, ok := s.api.(timelineAPI)
	if !ok {
		return core.Page[core.TimelineItem]{}, errors.New("issue timeline: not supported by the client")
	}
	size := q.PageSize
	if size <= 0 {
		size = 50
	}
	return api.ListIssueTimeline(ctx, q.Repo, q.Number, q.Cursor, size)
}

// EditComment sends the new body of the comment with node id.
func (s *Service) EditComment(ctx context.Context, id, body string) error {
	api, ok := s.api.(editAPI)
	if !ok {
		return errors.New("edit comment: not supported by the client")
	}
	_, err := api.EditIssueComment(ctx, id, body)
	return err
}
