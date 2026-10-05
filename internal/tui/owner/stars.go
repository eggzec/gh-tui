package owner

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// starList is the Stars tab: the repositories a user starred, the latest
// first, named by owner and name since they are of many owners.
type starList struct {
	q owners.StarsQuery
	tableTab
}

// newStarList returns the list of the repositories the user login starred.
func (s *Section) newStarList(login string) *starList {
	l := &starList{q: owners.StarsQuery{Login: login}}
	l.FullNames = true
	query := func(cursor string) owners.StarsQuery {
		q := l.q
		q.Cursor = cursor
		return q
	}
	svc := s.svc
	read := func(ctx context.Context, q owners.StarsQuery, again bool) (core.Page[core.Repo], error) {
		q.Again = again
		return svc.Stars(ctx, q)
	}
	render := func(r core.Repo, selected bool, _ int) string {
		return s.drawer().Row(l.Cols(), r, selected, s.dates, s.now)
	}
	l.Feed = feed.New(ui.FeedPages("owner.stars", query, read), render,
		s.feedOptions(feed.WithKey(func(r core.Repo) string { return r.Ref.String() }), "No stars yet.", "load the stars", login)...)
	return l
}

func (l *starList) fresh(svc Service) bool { return svc.FreshStars(l.q) }
