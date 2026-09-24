package issues

import (
	"context"
	"errors"
	"sync"

	tea "charm.land/bubbletea/v2"

	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
)

// readAhead reads the issues of the first rows and of the row under the
// cursor ahead, if they changed since the last message.
func (s *Section) readAhead() tea.Cmd {
	if s.ahead == nil || !s.started || !s.hasRepo {
		return nil
	}
	first := s.ahead.First(s.rowAt)
	it, ok := s.list.Selected()
	hover := s.ahead.Moved(commentsQuery(s.repo, it.Number), ok)
	switch {
	case first == nil:
		return hover
	case hover == nil:
		return first
	}
	return tea.Batch(first, hover)
}

// readIssue returns a read of the issue and the first comments that the
// modal opens with into the cache of svc.
func readIssue(svc Service) func(ctx context.Context, q issuesvc.CommentsQuery) error {
	return func(ctx context.Context, q issuesvc.CommentsQuery) error {
		// Both at once, since the modal waits for both.
		var (
			wg     sync.WaitGroup
			getErr error
		)
		wg.Go(func() { _, getErr = svc.Get(ctx, q.Repo, q.Number) })
		_, err := svc.Comments(ctx, q)
		wg.Wait()
		return errors.Join(getErr, err)
	}
}
