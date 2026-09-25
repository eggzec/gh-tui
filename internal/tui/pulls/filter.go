package pulls

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/facets"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// Facets is what the filter offers of a repository: its labels, and the
// people who work on it.
type Facets interface {
	Labels(ctx context.Context, repo core.RepoRef) ([]core.Label, error)
	People(ctx context.Context, q facets.PeopleQuery) ([]core.User, error)
}

// WithFacets offers the labels and people of the repository in the filter,
// read from f when the user opens their fields. Without it they are typed.
func WithFacets(f Facets) Option {
	return func(s *Section) { s.facets = f }
}

// tab is a state the list shows, as a tab in its header.
type tab struct {
	state core.State
	label string
}

// tabs are the tabs of the header, in the order ] goes through them. All
// lists every state.
var tabs = []tab{
	{core.StateOpen, "Open"},
	{core.StateClosed, "Closed"},
	{core.StateMerged, "Merged"},
	{"", "All"},
}

// prefetched are the states whose first pages are read ahead, which All
// shows again.
var prefetched = []core.State{core.StateOpen, core.StateClosed, core.StateMerged}

// tabLabel returns the label of the tab of state.
func tabLabel(state core.State) string {
	for _, t := range tabs {
		if t.state == state {
			return t.label
		}
	}
	return string(state)
}

// nextTab returns the state delta tabs away from state.
func nextTab(state core.State, delta int) core.State {
	i := 0
	for j, t := range tabs {
		if t.state == state {
			i = j
		}
	}
	n := len(tabs)
	return tabs[((i+delta)%n+n)%n].state
}

// defaultSort is the order of a list without a sort, which the query
// leaves out.
const defaultSort = "sort:updated-desc"

// spec returns the fields of the filter of the pull requests of repo.
func (s *Section) spec() filterform.Spec {
	states := make([]filterform.Item, len(tabs))
	for i, t := range tabs {
		states[i] = filterform.Item{Label: t.label, Value: string(t.state)}
	}
	person := func(key, label, qualifier string) filterform.Field {
		return filterform.Field{
			Key: key, Label: label, Kind: filterform.Person, Qualifier: qualifier,
			Options: []filterform.Item{ui.MeItem}, Load: s.loadPeople(), Hint: "anyone",
		}
	}
	return filterform.Spec{
		Fields: []filterform.Field{
			{
				Key: "state", Label: "State", Kind: filterform.Choice, Qualifier: "is",
				Options: states, Default: filterform.TextValue(string(core.StateOpen)),
			},
			person("author", "Author", "author"),
			person("assignee", "Assignee", "assignee"),
			{
				Key: "review", Label: "Review", Kind: filterform.Choice,
				Options: []filterform.Item{
					{Label: "Any"},
					{Label: "Requested from me", Value: "review-requested:@me"},
					{Label: "Reviewed by me", Value: "reviewed-by:@me"},
					{Label: "Approved", Value: "review:approved"},
					{Label: "Changes requested", Value: "review:changes_requested"},
				},
			},
			{
				Key: "labels", Label: "Labels", Kind: filterform.Multi, Qualifier: "label", Each: true,
				Load: s.loadLabels(), Hint: "any", Empty: "No labels in this repository.",
			},
			{Key: "drafts", Label: "Drafts", Kind: filterform.Toggle, Qualifier: "-is:draft", Hint: "hide drafts"},
			{Key: "base", Label: "Base", Kind: filterform.Text, Qualifier: "base", Hint: "any branch"},
		},
		Sort: &filterform.SortField{
			Options: []filterform.Item{
				{Label: "Updated", Value: "updated"},
				{Label: "Created", Value: "created"},
				{Label: "Comments", Value: "comments"},
			},
			Desc: "↓ newest first", Asc: "↑ oldest first",
			Default: filterform.Sort{By: "updated", Desc: true},
		},
	}
}

func (s *Section) loadLabels() filterform.Loader {
	if s.facets == nil {
		return nil
	}
	f, repo := s.facets, s.repo
	return func(ctx context.Context, _ string) ([]filterform.Item, error) {
		labels, err := f.Labels(ctx, repo)
		return ui.LabelItems(labels), err
	}
}

func (s *Section) loadPeople() filterform.Loader {
	if s.facets == nil {
		return nil
	}
	f, repo := s.facets, s.repo
	return func(ctx context.Context, text string) ([]filterform.Item, error) {
		users, err := f.People(ctx, facets.PeopleQuery{Repo: repo, Text: text})
		return ui.PersonItems(users), err
	}
}

// Filter implements ui.Filterable: the form opens on the tab shown and the
// filters in force.
func (s *Section) Filter() (ui.Filter, bool) {
	if !s.hasRepo {
		return ui.Filter{}, false
	}
	q := s.query
	if s.tab != "" {
		q = strings.TrimSpace("is:" + string(s.tab) + " " + q)
	}
	return ui.Filter{Spec: s.spec(), Query: q, Subject: s.repo.String()}, true
}

// ApplyFilter implements ui.Filterable. The state goes to the tabs, and
// the rest, without the default sort, is the filter.
func (s *Section) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	state := core.State(msg.Values["state"].Text())
	query := ui.Without(msg.Query, func(t filterform.Token) bool {
		if strings.EqualFold(t.Raw, defaultSort) {
			return true
		}
		return strings.EqualFold(t.Qualifier, "is") && isState(t.Value)
	})
	return s.show(state, query)
}

// isState reports whether v names a tab's state.
func isState(v string) bool {
	for _, t := range tabs {
		if t.state != "" && strings.EqualFold(v, string(t.state)) {
			return true
		}
	}
	return false
}

// show lists the pull requests in state that query selects, unless they
// are shown already.
func (s *Section) show(state core.State, query string) tea.Cmd {
	if !s.hasRepo || state == s.tab && query == s.query {
		return nil
	}
	s.tab = state
	if query != s.query {
		s.query, s.chips = query, ui.Chips(query)
	}
	s.others.Opened(s.listQuery(s.tab))
	return s.newFeed()
}

// Chips implements ui.Chipper: the filters in force, for the pane's title.
func (s *Section) Chips() string { return s.chips }

// Claims implements ui.Claimer: the section takes the keys that switch
// tabs once it shows a repository.
func (s *Section) Claims(msg tea.KeyPressMsg) bool {
	return s.hasRepo && s.feed != nil && (key.Matches(msg, s.keys.NextTab) || key.Matches(msg, s.keys.PrevTab))
}
