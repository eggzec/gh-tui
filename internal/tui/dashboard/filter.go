package dashboard

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/dashboard"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// read returns the repositories of o that are cached, from the first,
// without I/O.
func (t *repoTabs) read(o *owner) []core.Repo {
	q := o.q
	q.Cursor = ""
	p, _ := t.s.svc.CachedAllRepos(q, dashboard.MaxOwnerRepos)
	return p.Items
}

// filter returns the filter in force.
func (t *repoTabs) filter() *ownerui.Filter { return t.current().Filter() }

// spec returns the fields of the filter of the repositories, offering the
// languages of the tab on view.
func (t *repoTabs) spec() filterform.Spec { return ownerui.Spec(t.read(t.current()), t.filter()) }

// setFilter filters every tab with the filter of query, unless it does
// already, and lists them again.
func (t *repoTabs) setFilter(query string) tea.Cmd {
	if query == t.filter().Query() {
		return nil
	}
	f := ownerui.ParseFilter(query)
	// The reads ahead of the list as it was filtered stop.
	t.s.aheadRepos.Reset(t.s.ctx)
	cmds := make([]tea.Cmd, 0, len(t.tabs))
	for _, o := range t.tabs {
		o.SetFilter(&f)
		o.Feed.SetEmptyText(o.emptyText(&f, ui.KeyOf(t.s.icons, t.s.keys.ClearFilter)))
		if o.started {
			cmds = append(cmds, o.Feed.Reset())
		}
	}
	return tea.Batch(cmds...)
}

// Filter implements ui.Filterable while the repositories have the focus.
func (s *Section) Filter() (ui.Filter, bool) {
	if s.focus != reposPane {
		return ui.Filter{}, false
	}
	return ui.Filter{Spec: s.repos.spec(), Query: s.repos.filter().Query(), Subject: paneTitles[reposPane]}, true
}

// ApplyFilter implements ui.Filterable. The default sort is left out of
// the query, so that it alone filters nothing.
func (s *Section) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	cmd := s.repos.setFilter(ownerui.WithoutDefaultSort(msg.Query))
	s.render()
	return cmd
}
