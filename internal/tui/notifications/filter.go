package notifications

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// defaultQuery is the query of the default inbox, the unread threads.
const defaultQuery = "is:unread"

// filter is the filter of the inbox, read from its query of GitHub-like
// qualifiers, such as is:unread reason:mention repo:o/r type:pr. GitHub
// lists read threads or not; the rest is filtered from the pages it sends.
type filter struct {
	// query is the query the filter was read from.
	query string
	// all lists read threads too, which GitHub does for all=true.
	all bool
	// reason, repo and typ keep the threads with that reason, in that
	// repository, and about that type of subject, when they are set.
	reason string
	repo   string
	typ    string
}

// parseFilter reads query. Without is:unread it lists every thread, read or
// not. Tokens it doesn't know keep every thread.
func parseFilter(query string) filter {
	f := filter{query: query, all: true}
	for _, t := range filterform.Tokenize(query) {
		v := strings.ToLower(t.Value)
		switch strings.ToLower(t.Qualifier) {
		case "is":
			if v == "unread" {
				f.all = false
			}
		case "reason":
			f.reason = v
		case "repo":
			f.repo = v
		case "type":
			f.typ = v
		}
	}
	return f
}

// local reports whether the filter leaves out threads of the pages GitHub
// sends.
func (f *filter) local() bool { return f.reason != "" || f.repo != "" || f.typ != "" }

// keeps reports whether n passes the filter.
func (f *filter) keeps(n *core.Notification) bool {
	switch {
	case f.reason != "" && !strings.EqualFold(n.Reason, f.reason):
		return false
	case f.repo != "" && !strings.EqualFold(n.Repo.String(), f.repo):
		return false
	case f.typ != "" && typeName(n.Subject.Type) != f.typ:
		return false
	}
	return true
}

// apply returns the threads of items the filter keeps.
func (f *filter) apply(items []core.Notification) []core.Notification {
	if !f.local() {
		return items
	}
	out := make([]core.Notification, 0, len(items))
	for i := range items {
		if f.keeps(&items[i]) {
			out = append(out, items[i])
		}
	}
	return out
}

// types are the subject types the filter offers, by the name the query
// gives them.
var types = []struct {
	name string
	typ  core.SubjectType
	// label is how the form shows it.
	label string
}{
	{"pr", core.SubjectPullRequest, "Pull request"},
	{"issue", core.SubjectIssue, "Issue"},
	{"release", core.SubjectRelease, "Release"},
	{"ci", core.SubjectCheckSuite, "CI"},
	{"discussion", core.SubjectDiscussion, "Discussion"},
	{"commit", core.SubjectCommit, "Commit"},
}

// typeName is the name of t in the query, or t in lower case for a type
// the filter doesn't offer.
func typeName(t core.SubjectType) string {
	for _, it := range types {
		if it.typ == t {
			return it.name
		}
	}
	return strings.ToLower(string(t))
}

// reasons are the reasons GitHub gives, the ones most asked for first.
var reasons = []filterform.Item{
	{Label: "Any"},
	{Label: "Review requested", Value: "review_requested"},
	{Label: "Mention", Value: "mention"},
	{Label: "Team mention", Value: "team_mention"},
	{Label: "Assigned", Value: "assign"},
	{Label: "Author", Value: "author"},
	{Label: "Comment", Value: "comment"},
	{Label: "CI", Value: "ci_activity"},
	{Label: "State change", Value: "state_change"},
	{Label: "Subscribed", Value: "subscribed"},
	{Label: "Security alert", Value: "security_alert"},
}

// spec returns the fields of the filter.
func (s *Section) spec() filterform.Spec {
	f := s.filter()
	typeItems := make([]filterform.Item, 0, len(types)+1)
	typeItems = append(typeItems, filterform.Item{Label: "Any"})
	for _, t := range types {
		typeItems = append(typeItems, filterform.Item{Label: t.label, Value: t.name})
	}
	return filterform.Spec{Fields: []filterform.Field{
		{
			Key: "unread", Label: "Unread", Kind: filterform.Toggle, Qualifier: defaultQuery,
			Hint: "unread only", Default: filterform.BoolValue(true),
		},
		{Key: "reason", Label: "Reason", Kind: filterform.Choice, Qualifier: "reason", Options: withValue(reasons, f.reason)},
		{Key: "repo", Label: "Repository", Kind: filterform.Choice, Qualifier: "repo", Options: s.repoItems(f.repo)},
		{Key: "type", Label: "Type", Kind: filterform.Choice, Qualifier: "type", Options: withValue(typeItems, f.typ)},
	}}
}

// withValue returns items with v among them, so that the form shows a
// value typed in the query.
func withValue(items []filterform.Item, v string) []filterform.Item {
	if v == "" || slices.ContainsFunc(items, func(it filterform.Item) bool { return strings.EqualFold(it.Value, v) }) {
		return items
	}
	return append(slices.Clip(items), filterform.Item{Label: v, Value: v})
}

// repoItems offers the repositories of the threads cached for the inbox
// on view, the most recent first, and repo.
func (s *Section) repoItems(repo string) []filterform.Item {
	items := []filterform.Item{{Label: "Any"}}
	seen := map[string]bool{}
	q := s.query("")
	for range maxRepoPages {
		p, ok := s.svc.CachedList(q)
		if !ok {
			break
		}
		for i := range p.Items {
			r := p.Items[i].Repo.String()
			if k := strings.ToLower(r); !seen[k] {
				seen[k] = true
				items = append(items, filterform.Item{Label: r, Value: k})
			}
		}
		if p.Last() {
			break
		}
		q.Cursor = p.Next
	}
	return withValue(items, repo)
}

// maxRepoPages bounds the cached pages the filter looks through for
// repositories.
const maxRepoPages = 10

// filter returns the filter in force.
func (s *Section) filter() *filter {
	if f := s.filt.Load(); f != nil {
		return f
	}
	f := parseFilter(defaultQuery)
	return &f
}

// Filter implements ui.Filterable.
func (s *Section) Filter() (ui.Filter, bool) {
	return ui.Filter{Spec: s.spec(), Query: s.filter().query}, true
}

// ApplyFilter implements ui.Filterable.
func (s *Section) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	return s.setFilter(msg.Query)
}

// setFilter lists the threads that query keeps, unless it does already.
func (s *Section) setFilter(query string) tea.Cmd {
	query = strings.Join(strings.Fields(query), " ")
	if query == s.filter().query {
		return nil
	}
	f := parseFilter(query)
	s.filt.Store(&f)
	s.renderHeader()
	if !s.started {
		return nil
	}
	return s.feed.Reset()
}

// filtered reports whether the filter is other than the default inbox.
func (s *Section) filtered() bool { return s.filter().query != defaultQuery }

// chips names the filter in a few words, for the header.
func (f *filter) chips() string {
	parts := make([]string, 0, 3)
	for _, t := range filterform.Tokenize(f.query) {
		switch strings.ToLower(t.Qualifier) {
		case "is":
			if strings.EqualFold(t.Value, "unread") {
				continue
			}
			parts = append(parts, t.Raw)
		case "reason":
			parts = append(parts, shortReason(strings.ToLower(t.Value)))
		case "repo", "type":
			parts = append(parts, t.Value)
		default:
			parts = append(parts, t.Raw)
		}
	}
	return strings.Join(parts, " · ")
}

// listQuery is what GitHub is asked for the filter: read threads or not.
func (f *filter) listQuery(cursor string) notifications.ListQuery {
	return notifications.ListQuery{Filter: core.NotificationFilter{All: f.all}, Cursor: cursor}
}
