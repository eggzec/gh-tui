package pulls

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// readAhead tells the reads ahead where the cursor is, which read the
// window around it once it rests there.
func (s *Section) readAhead() tea.Cmd {
	if s.feed == nil {
		return nil
	}
	if s.feed.Len() == 0 && !s.feed.Settled() {
		// The list hasn't loaded.
		return s.ahead.Window(nil, 0)
	}
	return s.ahead.Window(s.rowAt, s.feed.Index())
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
