package releases

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// kindExtra is what the extras of releases are kept as, and extraSchema the
// version of extra, which holds them.
const (
	kindExtra   = "releaseextra"
	extraSchema = 2
)

// extra is a release's extra as it is kept: with the validator of the
// comparison, since the entry's own is that of the tag's commit.
type extra struct {
	core.ReleaseExtra
	CompareETag string
	// CompareTag is the tag the validator was got for.
	CompareTag string
}

// DefaultBranch returns the name of the default branch of repo, empty if it
// can't tell.
type DefaultBranch func(ctx context.Context, repo core.RepoRef) (string, error)

// WithDefaultBranch sets how the service learns the default branch of a
// repository, which the header compares the tag with. Without it the tag is
// compared with HEAD and the branch is left unnamed.
func WithDefaultBranch(f DefaultBranch) Option {
	return func(o *options) { o.defaultBranch = f }
}

// CachedExtra returns the extra of release id of repo if it is in memory,
// fresh or stale, without a request.
func (s *Service) CachedExtra(repo core.RepoRef, id int64) (core.ReleaseExtra, bool) {
	e, st := s.extras.Get(key(repo, id))
	out := e.Value.ReleaseExtra
	// Read from the release, not kept, so an edited body shows at once.
	if rel, found := s.CachedGet(repo, id); found {
		out.Mentions = markdown.Mentions(rel.Body)
	}
	return out, st != cache.Miss
}

// Extra returns what the header of release id of repo shows beyond the
// release: the commit its tag names, how far the default branch is from it
// and who the notes mention. Each of the two requests is conditional, and
// what an earlier session kept counts as cached. A tag that is gone or
// unrelated to the branch leaves AheadBy at -1, with no error.
func (s *Service) Extra(ctx context.Context, repo core.RepoRef, id int64) (core.ReleaseExtra, error) {
	rel, err := s.Get(ctx, repo, id)
	if err != nil {
		return core.ReleaseExtra{}, err
	}
	k := key(repo, id)
	s.keptExtras.Warm(s.extras, k, true)
	load := func(ctx context.Context, prev cache.Entry[extra], ok bool) (cache.Entry[extra], error) {
		return s.loadExtra(ctx, repo, rel, prev, ok)
	}
	e, err := fallback.Fetch(ctx, s.extras, s.keptExtras, k, fallback.None[extra], fallback.Keep(s.keptExtras, k, load))
	if err != nil {
		return core.ReleaseExtra{}, fmt.Errorf("get extra of release %d of %s: %w", id, repo, err)
	}
	out := e.Value.ReleaseExtra
	out.Mentions = markdown.Mentions(rel.Body)
	return out, nil
}

func (s *Service) loadExtra(ctx context.Context, repo core.RepoRef, rel core.Release, prev cache.Entry[extra], ok bool) (cache.Entry[extra], error) {
	var def string
	if s.defaultBranch != nil {
		// A failure leaves the branch unnamed rather than the header out.
		def, _ = s.defaultBranch(ctx, repo)
	}
	head := def
	if head == "" {
		head = "HEAD"
	}

	var condCommit, condCompare github.Conditional
	if ok {
		condCommit = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		// The comparison's validator says nothing of another branch, or of
		// another tag, as a release may be edited to name one.
		if prev.Value.Default == def && prev.Value.CompareTag == rel.Tag {
			condCompare = github.Conditional{ETag: prev.Value.CompareETag}
		}
	}

	var out extra
	out.Default, out.AheadBy = def, -1
	entry := cache.Entry[extra]{}
	commit, res, err := s.api.CommitOf(ctx, repo, rel.Tag, condCommit)
	var apiErr *github.Error
	if noCommit(err) {
		// A deleted tag, or the tag of a draft, which exists once it is
		// published: there is no commit to show and nothing to compare.
		// GitHub answers 422, not 404, for a ref it doesn't have. The entry
		// carries no validators, since an error answer has none, so it is
		// checked again once it is stale, and its lifetime is not widened.
		if ok && prev.Value.TagSHA == "" && prev.Value.AheadBy == -1 && prev.Value.Default == def {
			return cache.Entry[extra]{}, cache.ErrNotModified
		}
		return cache.Entry[extra]{Value: out}, nil
	}
	if err != nil {
		return cache.Entry[extra]{}, err
	}
	commitSame := res.NotModified
	if commitSame {
		out.TagSHA, out.Verified = prev.Value.TagSHA, prev.Value.Verified
		entry.ETag, entry.LastModified, entry.Source = prev.ETag, prev.LastModified, prev.Source
	} else {
		out.TagSHA, out.Verified = commit.SHA, commit.Verification.Verified
		entry.ETag, entry.LastModified, entry.Source = res.ETag, res.LastModified, res.URL
	}

	cmp, cres, err := s.api.Compare(ctx, repo, rel.Tag, head, condCompare)
	switch {
	case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
		// The tag is gone or has nothing to do with the branch.
	case err != nil:
		return cache.Entry[extra]{}, err
	case cres.NotModified:
		out.AheadBy, out.CompareETag, out.CompareTag = prev.Value.AheadBy, prev.Value.CompareETag, rel.Tag
	default:
		out.AheadBy, out.CompareETag, out.CompareTag = cmp.AheadBy, cres.ETag, rel.Tag
	}

	if ok && commitSame && out.AheadBy == prev.Value.AheadBy && out.Default == prev.Value.Default &&
		out.CompareETag == prev.Value.CompareETag && out.CompareTag == prev.Value.CompareTag {
		return cache.Entry[extra]{}, cache.ErrNotModified
	}
	entry.Value = out
	return entry, nil
}

// noCommit reports whether err says the tag has no commit: GitHub answers
// 404 for some missing refs and 422 for others.
func noCommit(err error) bool {
	var apiErr *github.Error
	return errors.As(err, &apiErr) &&
		(apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusUnprocessableEntity)
}
