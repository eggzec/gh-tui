package owner

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// repoList is the Repositories tab: the repositories of the account that
// the viewer can see, paged as the list scrolls. A filter lists those it
// keeps of up to owners.MaxAllRepos of them instead.
type repoList struct {
	q owners.ReposQuery
	tableTab
}

// tableTab is a tab that lists repositories in the columns of a table.
type tableTab struct {
	ownerui.Table
	// on is set once the feed has fetched its first page.
	on bool
}

func (l *tableTab) feed() feedModel { return &l.Feed }

func (l *tableTab) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.Feed, cmd = l.Feed.Update(msg)
	return cmd
}

func (l *tableTab) start() tea.Cmd {
	if l.on {
		return nil
	}
	l.on = true
	return l.Feed.Init()
}

func (l *tableTab) started() bool { return l.on }

func (l *tableTab) resize(s *Section, width, height int) {
	l.Feed.SetSize(width, max(height-listTop, 0))
	l.Layout(max(width-gutterWidth, 0), s.icons.Star, s.dates.Width())
}

func (l *tableTab) header(s *Section) string {
	if l.Feed.Len() == 0 {
		return ""
	}
	return strings.Repeat(" ", gutterWidth) + s.st.shared.Subtle.Render(l.Cols().Header(s.icons.Star, s.icons.Ellipsis))
}

func (l *tableTab) remeasure(s *Section) bool { return l.Remeasure(s.icons) }

func (l *tableTab) selection(s *Section) (ui.Selection, bool) {
	r, ok := l.Feed.Selected()
	if !ok {
		return ui.Selection{}, false
	}
	return ui.RepoSelection(r, s.repoURL(r)), true
}

func (l *tableTab) enter(*Section) tea.Cmd {
	r, ok := l.Feed.Selected()
	if !ok {
		return nil
	}
	return selectRepo(r.Ref)
}

func (l *repoList) fresh(svc Service) bool { return svc.FreshRepos(l.q) }

// newRepoList returns the list of the repositories of o, a user's or an
// organization's.
func (s *Section) newRepoList(o core.Owner) *repoList {
	l := &repoList{q: owners.ReposQuery{Owner: o.Profile.Login, Kind: o.Kind}}
	query := func(cursor string) owners.ReposQuery {
		q := l.q
		q.Cursor = cursor
		return q
	}
	svc := s.svc
	read := func(ctx context.Context, q owners.ReposQuery, again bool) (core.Page[core.Repo], error) {
		q.Again = again
		f := l.Filter()
		if !f.Active() {
			return svc.Repos(ctx, q)
		}
		// The filter runs over every repository, so that it sorts them
		// all. The pages the list read are cached and cost nothing.
		p, err := svc.AllRepos(ctx, q, owners.MaxAllRepos)
		p.Items, p.Next = f.Apply(p.Items), ""
		return p, err
	}
	render := func(r core.Repo, selected bool, _ int) string {
		return s.drawer().Row(l.Cols(), r, selected, s.dates, s.now)
	}
	l.Feed = feed.New(ui.FeedPages("owner.repos", query, read), render,
		feed.WithContext(s.ctx),
		feed.WithKey(func(r core.Repo) string { return r.Ref.String() }),
		feed.WithKeyMap(s.keys.feed),
		feed.WithPromptKeys(s.keys.search),
		feed.WithEmptyText(s.reposEmpty(l)),
		feed.WithStyles(s.theme.Feed(s.icons)),
		feed.WithErrorText(ui.ErrorText("load the repositories", o.Profile.Login, s.voice)),
	)
	return l
}

// reposEmpty is what the list l says when it has no repository.
func (s *Section) reposEmpty(l *repoList) string {
	if l.Filter().Active() {
		return ui.NoMatch("repositories", ui.KeyOf(s.icons, s.keys.ClearFilter))
	}
	return ui.None("repositories you can see")
}

// listTop is how many lines the tabs and the headers of the columns take
// above the list.
const listTop = 2

// gutterWidth is the room the list leaves for its cursor.
const gutterWidth = 2

// cachedRepos returns the repositories of l that are cached, from the first,
// without I/O, for the languages the filter offers.
func (s *Section) cachedRepos(l *repoList) []core.Repo {
	p, _ := s.svc.CachedAllRepos(l.q, owners.MaxAllRepos)
	return p.Items
}

// repoTab returns the repositories while their tab is on view, or nil.
func (s *Section) repoTab() *repoList {
	if s.page == nil || s.page.tab != reposTab {
		return nil
	}
	return s.page.repos()
}

// Filter implements ui.Filterable while the repositories have the focus.
func (s *Section) Filter() (ui.Filter, bool) {
	l := s.repoTab()
	if l == nil || s.page.focus != listPane {
		return ui.Filter{}, false
	}
	return ui.Filter{Spec: ownerui.Spec(s.cachedRepos(l), l.Filter()), Query: l.Filter().Query(), Subject: tabTitles[s.page.tab]}, true
}

// ApplyFilter implements ui.Filterable. The default sort is left out of
// the query, so that it alone filters nothing.
func (s *Section) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	cmd := s.setFilter(ownerui.WithoutDefaultSort(msg.Query))
	s.render()
	return cmd
}

// setFilter filters the repositories with the filter of query, unless it
// does already, and lists them again.
func (s *Section) setFilter(query string) tea.Cmd {
	l := s.repoTab()
	if l == nil || query == l.Filter().Query() {
		return nil
	}
	f := ownerui.ParseFilter(query)
	l.SetFilter(&f)
	l.Feed.SetEmptyText(s.reposEmpty(l))
	if !l.on {
		return nil
	}
	return l.Feed.Reset()
}
