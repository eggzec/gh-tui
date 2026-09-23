package notifications

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

var errUnexpected = errors.New("unexpected call")

// fakeAPI answers with its func fields and counts list calls. A nil field
// fails the call. OnMark, if set, runs before every mark.
type fakeAPI struct {
	list     func(filter core.NotificationFilter, perPage int, cursor string, cond github.Conditional) (page, github.Response, error)
	markRead func(id string) error
	markDone func(id string) error
	markAll  func(at time.Time) error
	onMark   func()

	lists atomic.Int32
}

func (f *fakeAPI) ListNotifications(_ context.Context, filter core.NotificationFilter, perPage int, cursor string, cond github.Conditional) (core.Page[core.Notification], github.Response, error) {
	f.lists.Add(1)
	if f.list == nil {
		return page{}, github.Response{}, errUnexpected
	}
	return f.list(filter, perPage, cursor, cond)
}

func (f *fakeAPI) MarkThreadRead(_ context.Context, id string) error {
	f.mark()
	if f.markRead == nil {
		return errUnexpected
	}
	return f.markRead(id)
}

func (f *fakeAPI) MarkThreadDone(_ context.Context, id string) error {
	f.mark()
	if f.markDone == nil {
		return errUnexpected
	}
	return f.markDone(id)
}

func (f *fakeAPI) MarkNotificationsRead(_ context.Context, at time.Time) error {
	f.mark()
	if f.markAll == nil {
		return errUnexpected
	}
	return f.markAll(at)
}

func (f *fakeAPI) mark() {
	if f.onMark != nil {
		f.onMark()
	}
}

// entry is a cached page as load stores it.
func entry(p page, lastModified string) cache.Entry[page] {
	return cache.Entry[page]{Value: p, LastModified: lastModified, Tags: []string{tag}}
}
