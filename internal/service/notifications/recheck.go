package notifications

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// SyncKey names changes to the inbox in sync events: those that Poll finds,
// and those that the checks of Kept do.
const SyncKey = "notifications"

// Kept lists the pages that the service keeps with validators, for a
// revalidator to check in the background. Each check is one conditional
// request, which costs no rate limit when nothing changed. A page that
// changed is cached and kept, and reports SyncKey. It reads the store, so
// call it where I/O is fine.
func (s *Service) Kept() []revalidate.Entry {
	return recheck.Entries(s.kept, kind, s.target)
}

func (s *Service) target(key string) (recheck.Target, bool) {
	q, ok := parseKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Check: func(ctx context.Context) revalidate.Result {
		load := s.fetcher(q)
		_, res := recheck.Check(ctx, s.cache, s.kept, key, SyncKey, load)
		return res
	}}, true
}

// parseKey returns the query that ListQuery.key made key of.
func parseKey(key string) (ListQuery, bool) {
	rest, ok := strings.CutPrefix(key, "notifications?")
	if !ok {
		return ListQuery{}, false
	}
	// The cursor is a URL with a query of its own, so it comes last.
	params, cursor, ok := strings.Cut(rest, "&cursor=")
	if !ok {
		return ListQuery{}, false
	}
	v, err := url.ParseQuery(params)
	if err != nil {
		return ListQuery{}, false
	}
	all, aerr := strconv.ParseBool(v.Get("all"))
	part, perr := strconv.ParseBool(v.Get("participating"))
	size, serr := strconv.Atoi(v.Get("page_size"))
	if aerr != nil || perr != nil || serr != nil {
		return ListQuery{}, false
	}
	return ListQuery{Filter: core.NotificationFilter{All: all, Participating: part}, PageSize: size, Cursor: cursor}, true
}

// fetcher reads the page for a normalized q, conditionally when there is a
// previous entry, without falling back to it or keeping what GitHub sends.
func (s *Service) fetcher(q ListQuery) cache.FetchFunc[page] {
	return recheck.Load(func(ctx context.Context, cond github.Conditional) (page, github.Response, error) {
		p, res, err := s.api.ListNotifications(ctx, q.Filter, q.PageSize, q.Cursor, cond)
		if err == nil && res.PollInterval > 0 {
			s.interval.Store(int64(res.PollInterval))
		}
		return p, res, err
	}, func(page) []string { return []string{tag} })
}
