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
	if s.ahead == nil || !s.live() {
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

// readOthers reads the first pages of the states not shown, once the list
// shown has loaded. A filtered list is the user's own search, which may
// cost more, so its other states wait until they are shown.
func (s *Section) readOthers() tea.Cmd {
	if s.others == nil || !s.live() || !s.list.Settled() || s.query != "" {
		return nil
	}
	return s.others.Read(func() []issuesvc.ListQuery {
		qs := make([]issuesvc.ListQuery, 0, len(tabs)-1)
		for _, t := range tabs {
			if t.state != s.tab {
				qs = append(qs, s.listQuery(t.state))
			}
		}
		return qs
	})
}

// readList returns a read of a first page into the cache of svc, which
// asks GitHub rather than serving a page an earlier session kept.
func readList(svc Service) func(ctx context.Context, q issuesvc.ListQuery) error {
	return func(ctx context.Context, q issuesvc.ListQuery) error {
		q.Again = true
		_, err := svc.List(ctx, q)
		return err
	}
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
