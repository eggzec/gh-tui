package search

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// PreviewAPI reads what the search preview of a user, an organization or a
// commit shows beyond its hit.
type PreviewAPI interface {
	GetUserProfile(ctx context.Context, login string) (core.UserProfile, error)
	GetCommit(ctx context.Context, repo core.RepoRef, sha string) (core.CommitDetail, error)
	CommitPatch(ctx context.Context, repo core.RepoRef, sha string) (patch string, tooLarge bool, err error)
}

// Previews reads those previews and keeps them in memory for a while, so
// that a hit seen again shows at once.
type Previews struct {
	api      PreviewAPI
	profiles *cache.Cache[core.UserProfile]
	commits  *cache.Cache[core.CommitPreview]
}

// NewPreviews returns Previews that read from api. WithTTL and WithCapacity
// apply; the others are ignored.
func NewPreviews(api PreviewAPI, opts ...Option) *Previews {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default()
	ttl := cmp.Or(o.ttl, d.Cache.TTL.RepoInfo)
	n := cmp.Or(o.capacity, d.Cache.Memory.Entries)
	return &Previews{
		api:      api,
		profiles: cache.New[core.UserProfile](cache.WithTTL(ttl), cache.WithCapacity(n)),
		commits:  cache.New[core.CommitPreview](cache.WithTTL(ttl), cache.WithCapacity(n)),
	}
}

// Profile returns the profile of the user or organization login.
func (p *Previews) Profile(ctx context.Context, login string) (core.UserProfile, error) {
	key := "profile/" + strings.ToLower(login)
	if e, state := p.profiles.Get(key); state == cache.Fresh {
		return e.Value, nil
	}
	out, err := p.api.GetUserProfile(ctx, login)
	if err != nil {
		return core.UserProfile{}, err
	}
	p.profiles.Set(key, cache.Entry[core.UserProfile]{Value: out})
	return out, nil
}

// Commit returns the commit sha of repo with its changed files and its
// patch. The two are read together, and the first failure is the error.
func (p *Previews) Commit(ctx context.Context, repo core.RepoRef, sha string) (core.CommitPreview, error) {
	key := "commit/" + strings.ToLower(repo.String()) + "@" + sha
	if e, state := p.commits.Get(key); state == cache.Fresh {
		return e.Value, nil
	}
	var out core.CommitPreview
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		out.Detail, err = p.api.GetCommit(ctx, repo, sha)
		return err
	})
	g.Go(func() (err error) {
		out.Patch, out.PatchTooLarge, err = p.api.CommitPatch(ctx, repo, sha)
		return err
	})
	if err := g.Wait(); err != nil {
		return core.CommitPreview{}, fmt.Errorf("preview commit %s: %w", sha, err)
	}
	// The patch is shown whole; what each file holds of it is not kept.
	files := make([]core.CommitFile, len(out.Detail.Files))
	for i, f := range out.Detail.Files {
		f.Patch = ""
		files[i] = f
	}
	out.Detail.Files = files
	p.commits.Set(key, cache.Entry[core.CommitPreview]{Value: out})
	return out, nil
}
