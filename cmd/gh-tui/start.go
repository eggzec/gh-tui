package main

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
)

// searchStart lists the repositories the search page offers before the
// user types: those pinned in the config, then the viewer's own.
func searchStart(repos repoLister, pinned []core.RepoRef) searchpage.Start {
	return func(ctx context.Context) ([]core.Repo, error) {
		p, err := repos.List(ctx, reposvc.ListQuery{})
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
