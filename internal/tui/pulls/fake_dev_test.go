package pulls

import (
	"context"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
)

// fakeDev is a dev.Service that answers from what the test sets.
type fakeDev struct {
	mu    sync.Mutex
	links core.PullLinks
	err   error
	// warm makes the links cached before any read.
	warm  bool
	reads []pulls.LinksQuery
}

var _ dev.Service = (*fakeDev)(nil)

func (f *fakeDev) CachedLinks(pulls.LinksQuery) (core.PullLinks, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.links, f.warm
}

func (f *fakeDev) Links(_ context.Context, q pulls.LinksQuery) (core.PullLinks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, q)
	return f.links, f.err
}

func (f *fakeDev) CachedIssuePreview(pulls.PreviewQuery) (core.IssuePreview, bool) {
	return core.IssuePreview{}, false
}

func (f *fakeDev) IssuePreview(context.Context, pulls.PreviewQuery) (core.IssuePreview, error) {
	return core.IssuePreview{}, nil
}

func (f *fakeDev) CachedPullPreview(pulls.PreviewQuery) (core.PullPreview, bool) {
	return core.PullPreview{}, false
}

func (f *fakeDev) PullPreview(context.Context, pulls.PreviewQuery) (core.PullPreview, error) {
	return core.PullPreview{}, nil
}
