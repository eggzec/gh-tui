package pulls

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// What changed since the user last looked at a pull request is a comparison
// of the head they saw with its head now. How far apart the two are can
// change, since the branch they name may move, so the counts are cached in
// memory and confirmed with their ETag once the TTL runs out. The files
// between two commits never change, so they are kept for good, by the SHAs.

// errNotSHA is a range, or the files of a commit, that is read by something
// other than the full SHAs, which are what make its files immutable.
var errNotSHA = errors.New("not a full commit SHA")

// SinceQuery selects the comparison of the head of a pull request now with
// the one the user saw.
type SinceQuery struct {
	Repo core.RepoRef
	// Base is the commit the user last saw, Head the commit now. Each is a
	// commit SHA, a branch or a tag.
	Base, Head string
}

// RangeQuery selects the files that changed between two commits.
type RangeQuery struct {
	Repo core.RepoRef
	// Base and Head are full commit SHAs, which is what lets the files be
	// kept for good.
	Base, Head string
}

func (q SinceQuery) key() string {
	return "pullsince:" + repoID(q.Repo) + ":" + q.Base + "..." + q.Head
}

func (q RangeQuery) key() string {
	return "pullrange:" + repoID(q.Repo) + ":" + strings.ToLower(q.Base) + "..." + strings.ToLower(q.Head)
}

// CachedSince returns the cached comparison for q, fresh or stale, without a
// request.
func (s *Service) CachedSince(q SinceQuery) (core.Compare, bool) {
	return cached(s.sinces, q.key())
}

// Since returns how far the head of q is from its base: the commits pushed
// since, and the commits the base has that the head lacks. A fresh
// comparison is returned without a request, and a stale one is confirmed
// with its ETag, which a 304 answers free of the rate limit. If GitHub can't
// be reached, the stale comparison is served.
//
// A comparison with a commit that a force push removed fails with
// core.ErrRewritten.
func (s *Service) Since(ctx context.Context, q SinceQuery) (core.Compare, error) {
	load := recheck.Load(func(ctx context.Context, cond github.Conditional) (core.Compare, github.Response, error) {
		return s.api.Compare(ctx, q.Repo, q.Base, q.Head, cond)
	}, func(core.Compare) []string { return []string{repoTag(q.Repo)} })
	e, err := fallback.Fetch(ctx, s.sinces, nil, q.key(), fallback.None[core.Compare], load)
	if err != nil {
		return core.Compare{}, fmt.Errorf("compare %s...%s of %s: %w", q.Base, q.Head, q.Repo, rewritten(err))
	}
	return e.Value, nil
}

// CachedRange returns the files of the range of q if they are in memory,
// without a request.
func (s *Service) CachedRange(q RangeQuery) (core.CompareDetail, bool) {
	return cached(s.ranges, q.key())
}

// RangeFiles returns the files that changed between the commits of q, with
// their patches and the counts of the comparison. The commits are named by
// their full SHAs, so the answer never changes: it is cached in memory and
// kept in the store for good, and read once. At most core.MaxCompareFiles
// files come, and Truncated says that more changed.
//
// A range with a commit that a force push removed fails with
// core.ErrRewritten.
func (s *Service) RangeFiles(ctx context.Context, q RangeQuery) (core.CompareDetail, error) {
	if !isSHA(q.Base) || !isSHA(q.Head) {
		return core.CompareDetail{}, fmt.Errorf("compare files %q...%q of %s: %w", q.Base, q.Head, q.Repo, errNotSHA)
	}
	key := q.key()
	e, err := s.ranges.Fetch(ctx, key, func(ctx context.Context, _ cache.Entry[core.CompareDetail], _ bool) (cache.Entry[core.CompareDetail], error) {
		if kept, ok := s.keptRanges.Load(key); ok {
			return cache.Entry[core.CompareDetail]{Value: kept.Value}, nil
		}
		d, _, err := s.api.CompareFiles(ctx, q.Repo, q.Base, q.Head, github.Conditional{})
		if err != nil {
			return cache.Entry[core.CompareDetail]{}, err
		}
		e := cache.Entry[core.CompareDetail]{Value: d}
		// The shelf is only a shortcut, so a failure is ignored.
		_ = s.keptRanges.Save(key, e)
		return e, nil
	})
	if err != nil {
		return core.CompareDetail{}, fmt.Errorf("compare files %s...%s of %s: %w", q.Base, q.Head, q.Repo, rewritten(err))
	}
	return e.Value, nil
}

// rewritten tags err as core.ErrRewritten if it says that one of the two
// commits isn't there: GitHub answers 404, or 422 for commits with no
// common ancestor, for the old head of a branch that was force pushed.
func rewritten(err error) error {
	if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrConflict) {
		return fmt.Errorf("%w: %w", core.ErrRewritten, err)
	}
	return err
}

// isSHA reports whether ref is a full SHA-1 or SHA-256 object name.
func isSHA(ref string) bool {
	if len(ref) != 40 && len(ref) != 64 {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil
}

// rangeSize is the memory a cached range takes, about.
func rangeSize(d core.CompareDetail) int64 {
	n := int64(128)
	for i := range d.Files {
		f := &d.Files[i]
		n += int64(len(f.Patch)+len(f.Path)+len(f.PreviousPath)+len(f.SHA)) + 64
	}
	return n
}
