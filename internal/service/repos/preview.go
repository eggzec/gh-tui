package repos

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

// PreviewAPI reads what the search preview of a repository shows beyond its
// hit.
type PreviewAPI interface {
	RepoReadme(ctx context.Context, ref core.RepoRef) (string, error)
	LatestRelease(ctx context.Context, ref core.RepoRef) (core.Release, bool, error)
	OpenCounts(ctx context.Context, ref core.RepoRef) (issues, pulls int, err error)
}

// Previews reads the previews of repositories and keeps them in memory for
// a while, so that a hit seen again shows at once.
type Previews struct {
	api   PreviewAPI
	cache *cache.Cache[core.RepoPreview]
}

// NewPreviews returns Previews that read from api. WithInfoTTL and
// WithCapacity apply; the others are ignored.
func NewPreviews(api PreviewAPI, opts ...Option) *Previews {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default()
	return &Previews{
		api: api,
		cache: cache.New[core.RepoPreview](cache.WithTTL(cmp.Or(o.infoTTL, d.Cache.TTL.RepoInfo)),
			cache.WithCapacity(cmp.Or(o.capacity, d.Cache.Memory.Entries))),
	}
}

// Preview returns the README, open counts and latest release of ref. A fresh
// cached preview is returned without a request. The three are read together,
// and the first failure is the error.
func (p *Previews) Preview(ctx context.Context, ref core.RepoRef) (core.RepoPreview, error) {
	key := "preview/" + strings.ToLower(ref.String())
	if e, state := p.cache.Get(key); state == cache.Fresh {
		return e.Value, nil
	}
	var out core.RepoPreview
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		out.Readme, err = p.api.RepoReadme(ctx, ref)
		return err
	})
	g.Go(func() error {
		rel, ok, err := p.api.LatestRelease(ctx, ref)
		if ok {
			out.Release = rel
		}
		return err
	})
	g.Go(func() (err error) {
		out.OpenIssues, out.OpenPulls, err = p.api.OpenCounts(ctx, ref)
		return err
	})
	if err := g.Wait(); err != nil {
		return core.RepoPreview{}, fmt.Errorf("preview %s: %w", ref, err)
	}
	p.cache.Set(key, cache.Entry[core.RepoPreview]{Value: out})
	return out, nil
}
