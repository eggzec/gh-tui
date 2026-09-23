package notifications

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

var errUnexpected = errors.New("unexpected call")

// fakeAPI answers with its func fields and counts list calls. A nil field
// fails the call.
type fakeAPI struct {
	list func(filter core.NotificationFilter, cursor string, cond github.Conditional) (page, github.Response, error)

	lists atomic.Int32
}

func (f *fakeAPI) ListNotifications(_ context.Context, filter core.NotificationFilter, cursor string, cond github.Conditional) (core.Page[core.Notification], github.Response, error) {
	f.lists.Add(1)
	if f.list == nil {
		return page{}, github.Response{}, errUnexpected
	}
	return f.list(filter, cursor, cond)
}

// entry is a cached page as load stores it.
func entry(p page, lastModified string) cache.Entry[page] {
	return cache.Entry[page]{Value: p, LastModified: lastModified, Tags: []string{tag}}
}
