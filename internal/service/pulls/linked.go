package pulls

import (
	"context"
	"fmt"
	"strconv"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// The issues a pull request closes, and the previews of an issue or a pull
// request, are read lazily and kept in memory only: what they show changes on
// the web, and a refresh of the pull request drops its links.

// linkedAPI is what the reads of linked items call.
type linkedAPI interface {
	PullLinks(ctx context.Context, repo core.RepoRef, number int) (core.PullLinks, error)
	IssuePreview(ctx context.Context, repo core.RepoRef, number int) (core.IssuePreview, error)
	PullPreview(ctx context.Context, repo core.RepoRef, number int) (core.PullPreview, error)
}

// LinksQuery selects the linked items of a pull request.
type LinksQuery struct {
	Repo   core.RepoRef
	Number int
}

// key is the key of the links of q.
func (q LinksQuery) key() string {
	return "pulllinks:" + repoID(q.Repo) + "#" + strconv.Itoa(q.Number)
}

// PreviewQuery selects the item to preview: an issue or a pull request of
// Repo, which may be another repository than the one on view.
type PreviewQuery struct {
	Repo   core.RepoRef
	Number int
}

// key is the key of the preview of q, of the kind of item that prefix names.
func (q PreviewQuery) key(prefix string) string {
	return prefix + ":" + repoID(q.Repo) + "#" + strconv.Itoa(q.Number)
}

// CachedLinks returns the links of q if they are cached, without fetching them.
func (s *Service) CachedLinks(q LinksQuery) (core.PullLinks, bool) {
	return cached(s.pullLinks, q.key())
}

// Links returns the issues that pull request q.Number closes, what blocks
// them, and the other pull requests that close the same issues. A fresh cached
// read is returned without a request, and a refresh of the pull request drops it.
func (s *Service) Links(ctx context.Context, q LinksQuery) (core.PullLinks, error) {
	e, err := s.pullLinks.Fetch(ctx, q.key(), func(ctx context.Context, _ cache.Entry[core.PullLinks], _ bool) (cache.Entry[core.PullLinks], error) {
		l, err := s.api.PullLinks(ctx, q.Repo, q.Number)
		if err != nil {
			return cache.Entry[core.PullLinks]{}, err
		}
		return cache.Entry[core.PullLinks]{Value: l, Tags: tags(q.Repo, q.Number)}, nil
	})
	if err != nil {
		return core.PullLinks{}, fmt.Errorf("read linked items of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return e.Value, nil
}

// CachedIssuePreview returns the preview of issue q if it is cached, without
// fetching it.
func (s *Service) CachedIssuePreview(q PreviewQuery) (core.IssuePreview, bool) {
	return cached(s.issuePreviews, q.key("issuepreview"))
}

// IssuePreview returns the short preview of issue q.Number of q.Repo. A fresh
// cached read is returned without a request.
func (s *Service) IssuePreview(ctx context.Context, q PreviewQuery) (core.IssuePreview, error) {
	e, err := s.issuePreviews.Fetch(ctx, q.key("issuepreview"), func(ctx context.Context, _ cache.Entry[core.IssuePreview], _ bool) (cache.Entry[core.IssuePreview], error) {
		p, err := s.api.IssuePreview(ctx, q.Repo, q.Number)
		if err != nil {
			return cache.Entry[core.IssuePreview]{}, err
		}
		return cache.Entry[core.IssuePreview]{Value: p, Tags: tags(q.Repo, q.Number)}, nil
	})
	if err != nil {
		return core.IssuePreview{}, fmt.Errorf("read preview of %s#%d: %w", q.Repo, q.Number, err)
	}
	return e.Value, nil
}

// CachedPullPreview returns the preview of pull request q if it is cached,
// without fetching it.
func (s *Service) CachedPullPreview(q PreviewQuery) (core.PullPreview, bool) {
	return cached(s.pullPreviews, q.key("pullpreview"))
}

// PullPreview returns the short preview of pull request q.Number of q.Repo. A
// fresh cached read is returned without a request.
func (s *Service) PullPreview(ctx context.Context, q PreviewQuery) (core.PullPreview, error) {
	e, err := s.pullPreviews.Fetch(ctx, q.key("pullpreview"), func(ctx context.Context, _ cache.Entry[core.PullPreview], _ bool) (cache.Entry[core.PullPreview], error) {
		p, err := s.api.PullPreview(ctx, q.Repo, q.Number)
		if err != nil {
			return cache.Entry[core.PullPreview]{}, err
		}
		return cache.Entry[core.PullPreview]{Value: p, Tags: tags(q.Repo, q.Number)}, nil
	})
	if err != nil {
		return core.PullPreview{}, fmt.Errorf("read preview of %s#%d: %w", q.Repo, q.Number, err)
	}
	return e.Value, nil
}
