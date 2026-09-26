package pulls

import (
	"context"
	"errors"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// readAhead reads the details of the first rows and of the row under the
// cursor ahead, if they changed since the last message.
func (s *Section) readAhead() tea.Cmd {
	if s.ahead == nil || s.feed == nil {
		return nil
	}
	first := s.ahead.First(s.rowAt)
	pr, ok := s.feed.Selected()
	hover := s.ahead.Moved(commentsQuery(s.repo, pr.Number), ok)
	switch {
	case first == nil:
		return hover
	case hover == nil:
		return first
	}
	return tea.Batch(first, hover)
}

// readOthers reads the first pages of the states not shown, once the list
// shown has loaded, if the user switched tabs in the repository before. A
// filtered list is the user's own search, which may cost more, so its
// other states wait until they are shown.
func (s *Section) readOthers() tea.Cmd {
	if s.others == nil || s.feed == nil || !s.feed.Settled() || s.query != "" {
		return nil
	}
	return s.others.Read(func() []pulls.ListQuery {
		qs := make([]pulls.ListQuery, 0, len(prefetched))
		for _, st := range prefetched {
			if st != s.tab {
				qs = append(qs, s.listQuery(st))
			}
		}
		return qs
	})
}

// readList returns a read of a first page into the cache of svc, which
// asks GitHub rather than serving a page an earlier session kept.
func readList(svc Service) func(ctx context.Context, q pulls.ListQuery) error {
	return func(ctx context.Context, q pulls.ListQuery) error {
		q.Again = true
		_, err := svc.List(ctx, q)
		return err
	}
}

// readDetail returns a read of the detail and the first comments that the
// modal opens with into the cache of svc.
func readDetail(svc Service) func(ctx context.Context, q pulls.CommentsQuery) error {
	return func(ctx context.Context, q pulls.CommentsQuery) error {
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
