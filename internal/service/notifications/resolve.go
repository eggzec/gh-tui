package notifications

import (
	"context"
	"log/slog"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
)

// resolveWorkers bounds the lookups that run at once for one page.
const resolveWorkers = 4

// needsResolve reports whether n names a release or check suite whose own
// page isn't known yet.
func needsResolve(n core.Notification) bool {
	s := n.Subject
	return !s.Resolved && (s.ReleaseID > 0 || s.CheckSuiteID > 0 || s.Type == core.SubjectCheckSuite)
}

// resolve sets the web URL of the releases and check suites in items to
// their own page, which GitHub doesn't say: it takes one read each. A thread
// that earlier already had its page resolved keeps it while the thread is
// unchanged, so the page is read once; GitHub reuses a thread for the next
// run of a workflow, with a later update time, and that is looked up again.
// A lookup that fails leaves the URL of the repository's list, and every
// later fetch that GitHub answers in full tries again.
func (s *Service) resolve(ctx context.Context, items, earlier []core.Notification) {
	known := make(map[string]core.Notification, len(earlier))
	for i := range earlier {
		if n := &earlier[i]; n.Subject.Resolved {
			known[n.ID] = *n
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, resolveWorkers)
	for i := range items {
		n := items[i]
		if !needsResolve(n) {
			continue
		}
		if k, ok := known[n.ID]; ok && k.Subject.URL == n.Subject.URL && k.Subject.Title == n.Subject.Title && k.UpdatedAt.Equal(n.UpdatedAt) {
			items[i].Subject.WebURL, items[i].Subject.Resolved = k.Subject.WebURL, true
			continue
		}
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			u, err := s.api.ResolveSubjectURL(ctx, n)
			if err != nil {
				slog.DebugContext(ctx, "resolve notification subject", "span", "service.notifications", "thread", n.ID, "err", err)
				return
			}
			// Each goroutine writes its own element.
			items[i].Subject.WebURL, items[i].Subject.Resolved = u, true
		})
	}
	wg.Wait()
}
