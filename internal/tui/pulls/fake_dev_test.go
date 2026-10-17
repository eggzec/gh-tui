package pulls

import (
	"context"
	"strings"
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
	// issueReads and pullReads are the numbers whose previews were read, and
	// body is how many lines the body of an issue has.
	issueReads, pullReads []int
	body                  int
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

// IssuePreview answers with a body of body lines, and counts the read.
func (f *fakeDev) IssuePreview(_ context.Context, q pulls.PreviewQuery) (core.IssuePreview, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issueReads = append(f.issueReads, q.Number)
	return core.IssuePreview{
		Reference: devRef(core.KindIssue, q.Number, "Issue title"),
		Author:    "ada", Body: strings.Repeat("a line of the body\n", f.body),
	}, nil
}

func (f *fakeDev) CachedPullPreview(pulls.PreviewQuery) (core.PullPreview, bool) {
	return core.PullPreview{}, false
}

func (f *fakeDev) PullPreview(_ context.Context, q pulls.PreviewQuery) (core.PullPreview, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pullReads = append(f.pullReads, q.Number)
	return core.PullPreview{Reference: devRef(core.KindPull, q.Number, "Pull title"), Author: "ada", Head: "fix", Base: "main"}, nil
}
