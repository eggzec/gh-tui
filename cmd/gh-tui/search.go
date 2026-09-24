package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	searchsvc "github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// searcher finds repositories, issues and pull requests on GitHub.
type searcher interface {
	Search(ctx context.Context, q searchsvc.Query) (core.Page[core.SearchHit], error)
}

// repoLister lists the viewer's repositories.
type repoLister interface {
	List(ctx context.Context, q reposvc.ListQuery) (core.Page[core.Repo], error)
}

// searchItems adapts the search and repository services to the app's
// search. A query with text searches GitHub. An empty one lists the pinned
// repositories, then the viewer's own, so there is something to open
// before typing.
func searchItems(s searcher, repos repoLister, pinned []core.RepoRef) picker.Search {
	return func(ctx context.Context, q picker.Query) ([]picker.Item, error) {
		kind := scopeKind(q.Scope)
		if strings.TrimSpace(q.Text) == "" {
			return startItems(ctx, repos, pinned, kind)
		}
		p, err := s.Search(ctx, searchsvc.Query{Text: q.Text, Kind: kind})
		if err != nil {
			return nil, friendly(err)
		}
		items := make([]picker.Item, 0, len(p.Items))
		for i := range p.Items {
			if it, ok := hitItem(&p.Items[i]); ok {
				items = append(items, it)
			}
		}
		return items, nil
	}
}

func scopeKind(scope string) core.SearchKind {
	switch scope {
	case tui.KindRepos:
		return core.SearchRepos
	case tui.KindIssues:
		return core.SearchIssues
	case tui.KindPulls:
		return core.SearchPulls
	}
	return core.SearchAll
}

// startItems lists the repositories offered before any query.
func startItems(ctx context.Context, repos repoLister, pinned []core.RepoRef, kind core.SearchKind) ([]picker.Item, error) {
	if kind != core.SearchAll && kind != core.SearchRepos {
		return nil, nil
	}
	p, err := repos.List(ctx, reposvc.ListQuery{})
	if err != nil {
		return nil, fmt.Errorf("list your repositories: %w", friendly(err))
	}
	items := make([]picker.Item, 0, len(pinned)+len(p.Items))
	seen := make(map[core.RepoRef]bool, len(pinned))
	for _, ref := range pinned {
		seen[ref] = true
		items = append(items, picker.Item{Kind: tui.KindPinned, Title: ref.String(), Value: ref})
	}
	for i := range p.Items {
		if r := &p.Items[i]; !seen[r.Ref] {
			items = append(items, repoItem(r))
		}
	}
	return items, nil
}

func hitItem(hit *core.SearchHit) (picker.Item, bool) {
	switch hit.Kind {
	case core.SearchRepos:
		return repoItem(&hit.Repo), true
	case core.SearchIssues:
		return issueItem(tui.KindIssues, hit), true
	case core.SearchPulls:
		return issueItem(tui.KindPulls, hit), true
	default:
		return picker.Item{}, false
	}
}

func repoItem(r *core.Repo) picker.Item {
	return picker.Item{Kind: tui.KindRepos, Title: r.Ref.String(), Detail: r.Description, Value: *r}
}

func issueItem(kind string, hit *core.SearchHit) picker.Item {
	is := &hit.Issue
	return picker.Item{
		Kind:   kind,
		Title:  is.Title,
		Detail: is.Repo.String() + "#" + strconv.Itoa(is.Number),
		Value:  *hit,
	}
}

// friendly rewords a rate-limit error, which the search hits soonest: GitHub
// allows 30 searches a minute.
func friendly(err error) error {
	if !errors.Is(err, core.ErrRateLimited) {
		return err
	}
	if rl, ok := errors.AsType[*core.RateLimitError](err); ok && !rl.Reset.IsZero() {
		return fmt.Errorf("GitHub asks to slow down; try again at %s", rl.Reset.Local().Format(time.Kitchen))
	}
	return errors.New("GitHub asks to slow down; try again in a minute")
}
