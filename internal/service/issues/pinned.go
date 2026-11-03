package issues

import (
	"context"
	"fmt"
	"slices"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// pinnedKey is the key of the pinned issues of repo. They are a slice of
// their own, apart from the pages of the list, and only memory holds them.
func pinnedKey(repo core.RepoRef) string {
	return "pinned:" + repoID(repo)
}

// pinnedTags tags the pinned issues with their repository and each issue, so
// that a change to one of them finds them.
func pinnedTags(repo core.RepoRef) func([]core.Issue) []string {
	return func(items []core.Issue) []string {
		tags := make([]string, 0, len(items)+1)
		tags = append(tags, repoTag(repo))
		for i := range items {
			tags = append(tags, issueKey(repo, items[i].Number))
		}
		return tags
	}
}

// CachedPinned returns the pinned issues of repo in pin order, fresh or
// stale, without a request. It reports false if they aren't cached.
func (s *Service) CachedPinned(repo core.RepoRef) ([]core.Issue, bool) {
	e, st := s.pinned.Get(pinnedKey(repo))
	if st == cache.Miss {
		return nil, false
	}
	return slices.Clone(e.Value), true
}

// Pinned returns the pinned issues of repo in pin order, from the cache
// while it is fresh, else from GitHub.
func (s *Service) Pinned(ctx context.Context, repo core.RepoRef) ([]core.Issue, error) {
	key := pinnedKey(repo)
	if e, st := s.pinned.Get(key); st == cache.Fresh {
		return slices.Clone(e.Value), nil
	}
	items, err := s.api.PinnedIssues(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("pinned issues of %s: %w", repo, err)
	}
	items = slices.Clone(items)
	s.pinned.Set(key, cache.Entry[[]core.Issue]{Value: items, Tags: pinnedTags(repo)(items)})
	return slices.Clone(items), nil
}
