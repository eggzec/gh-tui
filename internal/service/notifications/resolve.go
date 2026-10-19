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
// that earlier already had its page resolved keeps it, so the page is read
// once. A lookup that fails leaves the URL of the repository's list, and
// the next fetch tries again.
func (s *Service) resolve(ctx context.Context, items, earlier []core.Notification) {
	known := make(map[string]core.Subject, len(earlier))
	for i := range earlier {
		if n := &earlier[i]; n.Subject.Resolved {
			known[n.ID] = n.Subject
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, resolveWorkers)
	for i := range items {
		n := items[i]
		if !needsResolve(n) {
			continue
		}
		if k, ok := known[n.ID]; ok && k.URL == n.Subject.URL {
			items[i].Subject.WebURL, items[i].Subject.Resolved = k.WebURL, true
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
