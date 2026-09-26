package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
)

// repoLister lists the viewer's repositories.
type repoLister interface {
	List(ctx context.Context, q reposvc.ListQuery) (core.Page[core.Repo], error)
}

// searchStart lists the repositories the search page offers before the
// user types: those pinned in the config, then the viewer's own. The first
// list may be the one an earlier session kept, which the search page shows
// as it is, so every later one reads past it.
func searchStart(repos repoLister, pinned []core.RepoRef) searchpage.Start {
	var listed atomic.Bool
	return func(ctx context.Context) ([]core.Repo, error) {
		p, err := repos.List(ctx, reposvc.ListQuery{Again: listed.Swap(true)})
		if err != nil {
			return nil, fmt.Errorf("list your repositories: %w", friendly(err))
		}
		out := make([]core.Repo, 0, len(pinned)+len(p.Items))
		seen := make(map[core.RepoRef]bool, len(pinned))
		for _, ref := range pinned {
			seen[ref] = true
			out = append(out, core.Repo{Ref: ref})
		}
		for i := range p.Items {
			if !seen[p.Items[i].Ref] {
				out = append(out, p.Items[i])
			}
		}
		return out, nil
	}
}

// friendly rewords a rate-limit error.
func friendly(err error) error {
	if !errors.Is(err, core.ErrRateLimited) {
		return err
	}
	if rl, ok := errors.AsType[*core.RateLimitError](err); ok && !rl.Reset.IsZero() {
		return fmt.Errorf("GitHub asks to slow down; try again at %s", rl.Reset.Local().Format(time.Kitchen))
	}
	return errors.New("GitHub asks to slow down; try again in a minute")
}
