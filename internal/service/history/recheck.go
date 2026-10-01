package history

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// Kept lists the pages of branches and the first pages of commits of refs
// that the service keeps with validators, for a revalidator to check in
// the background. Each check is one conditional request, which costs no
// rate limit when nothing moved. What changed is cached and kept, and
// reports SyncKey of its repository. What a SHA names never changes, so it
// isn't listed. It reads the store, so call it where I/O is fine.
func (s *Service) Kept() []revalidate.Entry {
	return slices.Concat(
		recheck.Entries(s.keptBranches, kindBranches, s.ttl, s.branchesTarget),
		recheck.Entries(s.keptRefPages, kindRefPages, s.ttl, s.refPageTarget),
	)
}

func (s *Service) branchesTarget(key string) (recheck.Target, bool) {
	q, ok := parseBranchesKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		load := recheck.Load(s.loadBranches(q), func(core.Page[core.Branch]) []string { return []string{repoTag(q.Repo)} })
		_, res := recheck.Check(ctx, s.branches, s.keptBranches, key, SyncKey(q.Repo), load)
		return res
	}}, true
}

func (s *Service) refPageTarget(key string) (recheck.Target, bool) {
	q, ok := parseRefPageKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		load := recheck.Load(s.loadRefPage(q), func(core.Page[core.Commit]) []string { return []string{repoTag(q.Repo)} })
		_, res := recheck.Check(ctx, s.refPages, s.keptRefPages, key, SyncKey(q.Repo), load)
		return res
	}}, true
}

// parseBranchesKey returns the query that branchesKey made key of.
func parseBranchesKey(key string) (BranchesQuery, bool) {
	repo, size, rest, ok := parseKey(key, "branches")
	return BranchesQuery{Repo: repo, PageSize: size, Cursor: rest}, ok
}

// parseRefPageKey returns the query that refPageKey made key of.
func parseRefPageKey(key string) (CommitsQuery, bool) {
	repo, size, rest, ok := parseKey(key, "commits")
	return CommitsQuery{Repo: repo, PageSize: size, Ref: rest}, ok && !isSHA(rest)
}

// parseKey parses "prefix:owner/name:size:rest".
func parseKey(key, prefix string) (repo core.RepoRef, size int, rest string, ok bool) {
	parts := strings.SplitN(key, ":", 4)
	if len(parts) != 4 || parts[0] != prefix {
		return core.RepoRef{}, 0, "", false
	}
	repo, err := core.ParseRepoRef(parts[1])
	size, serr := strconv.Atoi(parts[2])
	if err != nil || serr != nil || size <= 0 {
		return core.RepoRef{}, 0, "", false
	}
	return repo, size, parts[3], true
}
