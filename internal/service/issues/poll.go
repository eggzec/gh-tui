package issues

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/watch"
)

// SyncKey is the sync key under which the app subscribes Poll(repo).
// GitHub ignores case in owner and repository names, so the key does too.
func SyncKey(repo core.RepoRef) string {
	return "issues:" + repoID(repo)
}

// Poll returns a watch.PollFunc that asks GitHub, with a conditional
// request, whether any issue of repo changed. A 304 costs no rate limit and
// is no change. The first poll only learns the current ETag. When a later
// poll brings a new one, everything cached of repo is invalidated before
// Changed is reported, so the reads after the event revalidate with GitHub.
// GitHub counts pull requests as issues, so a changed pull request is
// reported too.
func (s *Service) Poll(repo core.RepoRef) watch.PollFunc {
	probe := func(ctx context.Context, cond github.Conditional) (github.Response, error) {
		res, err := s.api.ProbeIssues(ctx, repo, cond)
		if err != nil {
			return res, fmt.Errorf("poll issues of %s: %w", repo, err)
		}
		return res, nil
	}
	return s.etags.Poll(SyncKey(repo), probe, func() { s.Invalidate(repo) })
}
