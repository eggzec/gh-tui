// Package details reads pull requests and issues ahead of their modals,
// for the lists outside the repository screen that open them, such as the
// work on the dashboard, the results of the search and the notification
// threads: the detail and the first comments, as the modals read them, so
// that they open at once.
package details

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Pulls is what a read ahead needs of the pull requests service.
type Pulls interface {
	Get(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	Comments(ctx context.Context, q pulls.CommentsQuery) (core.Page[core.Comment], error)
	Current(q pulls.CommentsQuery) bool
}

// Issues is what a read ahead needs of the issues service.
type Issues interface {
	Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error)
	Comments(ctx context.Context, q issuesvc.CommentsQuery) (core.Page[core.Comment], error)
	Current(q issuesvc.CommentsQuery) bool
}

// Key names a pull request or an issue.
type Key struct {
	Pull   bool
	Repo   core.RepoRef
	Number int
}

// Of returns the key of the pull request or issue of hit, and false for a
// repository.
func Of(hit core.SearchHit) (Key, bool) {
	switch hit.Kind {
	case core.SearchPulls:
		return Key{Pull: true, Repo: hit.Issue.Repo, Number: hit.Issue.Number}, true
	case core.SearchIssues:
		return Key{Repo: hit.Issue.Repo, Number: hit.Issue.Number}, true
	default:
		return Key{}, false
	}
}

// NewAhead returns a ui.Ahead that reads through ps and is, counted under
// kind in the log: the first rows of a list, and the row the cursor rests
// on for delay. Each costs two requests, and what is cached is skipped. A
// nil service reads nothing of its kind.
func NewAhead(kind string, ps Pulls, is Issues, rows int, delay time.Duration) *ui.Ahead[Key] {
	r := Reader{Pulls: ps, Issues: is}
	return ui.NewAhead(kind, r.Read, r.Current, rows, delay)
}

// Reader reads pull requests and issues into the caches their modals read
// from, through Pulls and Issues.
type Reader struct {
	Pulls  Pulls
	Issues Issues
}

// Current reports whether the modal of k would open without a request, or
// there is no service to read it with. It does no I/O.
func (r Reader) Current(k Key) bool {
	if k.Pull {
		return r.Pulls == nil || r.Pulls.Current(pulls.CommentsQuery{Repo: k.Repo, Number: k.Number})
	}
	return r.Issues == nil || r.Issues.Current(issuesvc.CommentsQuery{Repo: k.Repo, Number: k.Number})
}

// Read reads the detail and the first comments of k into the cache its
// modal reads from.
func (r Reader) Read(ctx context.Context, k Key) error {
	if k.Pull {
		q := pulls.CommentsQuery{Repo: k.Repo, Number: k.Number}
		return both(
			func() error { _, err := r.Pulls.Get(ctx, k.Repo, k.Number); return err },
			func() error { _, err := r.Pulls.Comments(ctx, q); return err })
	}
	q := issuesvc.CommentsQuery{Repo: k.Repo, Number: k.Number}
	return both(
		func() error { _, err := r.Issues.Get(ctx, k.Repo, k.Number); return err },
		func() error { _, err := r.Issues.Comments(ctx, q); return err })
}

// both runs a and b at once, since the modal waits for both.
func both(a, b func() error) error {
	var (
		wg   sync.WaitGroup
		aErr error
	)
	wg.Go(func() { aErr = a() })
	bErr := b()
	wg.Wait()
	return errors.Join(aErr, bErr)
}
