package issues

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

// Facets is what the filter offers of a repository: its labels, open
// milestones and the people who work on it.
type Facets interface {
	Labels(ctx context.Context, repo core.RepoRef) ([]core.Label, error)
	People(ctx context.Context, q facets.PeopleQuery) ([]core.User, error)
	// CachedMilestones never does I/O, since the form lists the
	// milestones as it opens; Milestones reads them ahead.
	CachedMilestones(repo core.RepoRef) ([]core.Milestone, bool)
	Milestones(ctx context.Context, repo core.RepoRef) ([]core.Milestone, error)
}

// WithFacets offers the labels, milestones and people of the repository in
// the filter. The labels and people are read when the user opens their
// fields, and the milestones once the list loads. Without it they are
// typed.
func WithFacets(f Facets) Option {
	return func(s *Section) { s.facets = f }
}

// tab is a state the list shows, as a tab in its bar.
type tab struct {
	state core.StateFilter
	label string
}

// tabs are the tabs of the bar, in the order ] goes through them.
var tabs = []tab{
	{core.FilterOpen, "Open"},
	{core.FilterClosed, "Closed"},
	{core.FilterAll, "All"},
}

func tabLabel(state core.StateFilter) string {
	for _, t := range tabs {
		if t.state == state {
			return t.label
		}
	}
	return string(state)
}

// nextTab returns the state delta tabs away from state.
func nextTab(state core.StateFilter, delta int) core.StateFilter {
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

// noMilestone is the value of the milestone field for issues without one.
const noMilestone = "no:milestone"

// maxMilestones is how many milestones the filter offers, the ones due
// soonest; others are typed.
const maxMilestones = 6

// spec returns the fields of the filter of the issues of the repository.
func (s *Section) spec() filterform.Spec {
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
				Options: []filterform.Item{{Label: "Open", Value: "open"}, {Label: "Closed", Value: "closed"}, {Label: "All"}},
				Default: filterform.TextValue("open"),
			},
			{
				Key: "labels", Label: "Labels", Kind: filterform.Multi, Qualifier: "label", Each: true,
				Load: s.loadLabels(), Hint: "any", Empty: "No labels in this repository.",
			},
			person("assignee", "Assignee", "assignee"),
			person("author", "Author", "author"),
			{
				Key: "milestone", Label: "Milestone", Kind: filterform.Choice, Qualifier: "milestone",
				Options: s.milestoneItems(), Format: writeMilestone, Parse: claimMilestone,
			},
			person("mentions", "Mentions", "mentions"),
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

// milestoneItems returns the options of the milestone field: any, none,
// and the open milestones read so far.
func (s *Section) milestoneItems() []filterform.Item {
	items := []filterform.Item{{Label: "Any"}, {Label: "None", Value: noMilestone}}
	if s.facets == nil {
		return items
	}
	ms, _ := s.facets.CachedMilestones(s.repo)
	for _, m := range ms[:min(len(ms), maxMilestones)] {
		items = append(items, filterform.Item{Label: m.Title, Value: m.Title})
	}
	return items
}

func writeMilestone(v filterform.Value) string {
	switch t := v.Text(); t {
	case "", noMilestone:
		return t
	default:
		if strings.ContainsAny(t, " ,") {
			t = `"` + t + `"`
		}
		return "milestone:" + t
	}
}

func claimMilestone(tok filterform.Token, v filterform.Value) (filterform.Value, bool) {
	switch {
	case strings.EqualFold(tok.Raw, noMilestone):
		return filterform.TextValue(noMilestone), true
	case strings.EqualFold(tok.Qualifier, "milestone") && tok.Value != "":
		return filterform.TextValue(tok.Value), true
	}
	return v, false
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

// readMilestones reads the milestones of the repository ahead, once its
// list has loaded, so that the filter lists them as it opens.
func (s *Section) readMilestones() tea.Cmd {
	if s.facets == nil || s.milestonesRead || !s.started || !s.hasRepo || !s.list.Settled() {
		return nil
	}
	s.milestonesRead = true
	f, ctx, repo := s.facets, s.ctx, s.repo
	return func() tea.Msg {
		// The filter offers none if this fails, and they can be typed.
		_, _ = f.Milestones(ctx, repo)
		return nil
	}
}

// Filter implements ui.Filterable: the form opens on the tab shown and the
// filters in force.
func (s *Section) Filter() (ui.Filter, bool) {
	if !s.hasRepo {
		return ui.Filter{}, false
	}
	q := s.query
	if s.tab != core.FilterAll {
		q = strings.TrimSpace("is:" + string(s.tab) + " " + q)
	}
	return ui.Filter{Spec: s.spec(), Query: q, Subject: s.repo.String()}, true
}

// ApplyFilter implements ui.Filterable. The state goes to the tabs, and
// the rest, without the default sort, is the filter.
func (s *Section) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	state := core.StateFilter(msg.Values["state"].Text())
	if state == "" {
		state = core.FilterAll
	}
	query := ui.Without(msg.Query, func(t filterform.Token) bool {
		if strings.EqualFold(t.Raw, defaultSort) {
			return true
		}
		return strings.EqualFold(t.Qualifier, "is") && (strings.EqualFold(t.Value, "open") || strings.EqualFold(t.Value, "closed"))
	})
	return s.show(state, query)
}

// show lists the issues in state that query selects, unless they are
// shown already.
func (s *Section) show(state core.StateFilter, query string) tea.Cmd {
	if !s.hasRepo || state == s.tab && query == s.query {
		return nil
	}
	s.tab = state
	if query != s.query {
		s.query, s.filterChips = query, ui.Chips(query)
	}
	s.others.Opened(s.listQuery(s.tab))
	return s.resetList()
}

// Chips implements ui.Chipper: the filters in force, for the pane's title.
func (s *Section) Chips() string { return s.filterChips }

// Claims implements ui.Claimer: the section takes the keys that switch
// tabs once it shows a repository.
func (s *Section) Claims(msg tea.KeyPressMsg) bool {
	return s.hasRepo && (key.Matches(msg, s.keys.NextTab) || key.Matches(msg, s.keys.PrevTab))
}
