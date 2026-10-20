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
	// participating lists only the threads the user takes part in, which
	// GitHub does for participating=true.
	participating bool
	// since is the window of time, one of sinceWindows, that GitHub keeps
	// the threads updated within. Empty keeps every thread.
	since string
	// reasons, owners, repos and typs keep the threads with one of those
	// reasons, in a repository of one of those owners, in one of those
	// repositories, and about one of those types of subject, when they are
	// set. Several values of a key are alternatives; the keys all apply.
	reasons []string
	owners  []string
	repos   []string
	typs    []string
}

// parseFilter reads query. Without is:unread it lists every thread, read or
// not. Tokens it doesn't know keep every thread.
func parseFilter(query string) filter {
	f := filter{query: query, all: true}
	for _, t := range filterform.Tokenize(query) {
		v := strings.ToLower(t.Value)
		switch strings.ToLower(t.Qualifier) {
		case "is":
			switch v {
			case "unread":
				f.all = false
			case "participating":
				f.participating = true
			}
		case "since":
			if slices.ContainsFunc(sinceWindows, func(it filterform.Item) bool { return it.Value == v }) {
				f.since = v
			}
		case "reason":
			f.reasons = appendValues(f.reasons, t)
		case "owner":
			f.owners = appendValues(f.owners, t)
		case "repo":
			f.repos = appendValues(f.repos, t)
		case "type":
			f.typs = appendValues(f.typs, t)
		}
	}
	return f
}

// appendValues adds the values of t, in lower case, to list.
func appendValues(list []string, t filterform.Token) []string {
	for _, v := range t.Values() {
		if v = strings.ToLower(v); !slices.Contains(list, v) {
			list = append(list, v)
		}
	}
	return list
}

// local reports whether the filter leaves out threads of the pages GitHub
// sends.
func (f *filter) local() bool {
	return len(f.reasons) > 0 || len(f.owners) > 0 || len(f.repos) > 0 || len(f.typs) > 0
}

// keeps reports whether n passes the filter.
func (f *filter) keeps(n *core.Notification) bool {
	return oneOf(f.reasons, n.Reason) &&
		oneOf(f.owners, n.Repo.Owner) &&
		oneOf(f.repos, n.Repo.String()) &&
		oneOf(f.typs, typeName(n.Subject.Type))
}

// oneOf reports whether want is empty or has v, ignoring case.
func oneOf(want []string, v string) bool {
	return len(want) == 0 || slices.ContainsFunc(want, func(w string) bool { return strings.EqualFold(w, v) })
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
	{Label: "Approval requested", Value: "approval_requested"},
	{Label: "Invitation", Value: "invitation"},
	{Label: "Manual", Value: "manual"},
	{Label: "Member feature requested", Value: "member_feature_requested"},
	{Label: "Security advisory credit", Value: "security_advisory_credit"},
}

// sinceWindows are the windows of time the filter offers for the threads
// updated within them; the first, any, writes nothing.
var sinceWindows = []filterform.Item{
	{Label: "Any"},
	{Label: "1 day", Value: "1d"},
	{Label: "1 week", Value: "1w"},
	{Label: "1 month", Value: "1m"},
}

// spec returns the fields of the filter.
func (s *Section) spec() filterform.Spec {
	f := s.filter()
	typeItems := make([]filterform.Item, 0, len(types))
	for _, t := range types {
		typeItems = append(typeItems, filterform.Item{Label: t.label, Value: t.name})
	}
	owners, repos := s.repoItems()
	return filterform.Spec{Fields: []filterform.Field{
		{
			Key: "unread", Label: "Unread", Kind: filterform.Toggle, Qualifier: defaultQuery,
			Hint: "unread only", Default: filterform.BoolValue(true),
		},
		{
			Key: "participating", Label: "Participating", Kind: filterform.Toggle, Qualifier: "is:participating",
			Hint: "threads you take part in",
		},
		{
			Key: "since", Label: "Updated since", Kind: filterform.Choice, Qualifier: "since",
			Options: sinceWindows,
		},
		multi("reason", "Reason", withValues(reasons, f.reasons)),
		multi("owner", "Owner", withValues(owners, f.owners)),
		multi("repo", "Repository", withValues(repos, f.repos)),
		multi("type", "Type", withValues(typeItems, f.typs)),
	}}
}

// multi is a field that picks any number of items, one qualifier of its
// key each.
func multi(key, label string, items []filterform.Item) filterform.Field {
	return filterform.Field{
		Key: key, Label: label, Kind: filterform.Multi, Qualifier: key, Each: true,
		Options: items, Hint: "any",
	}
}

// withValues returns items with each of vs among them, so that the form
// shows the values typed in the query.
func withValues(items []filterform.Item, vs []string) []filterform.Item {
	for _, v := range vs {
		if slices.ContainsFunc(items, func(it filterform.Item) bool { return strings.EqualFold(it.Value, v) }) {
			continue
		}
		items = append(slices.Clip(items), filterform.Item{Label: v, Value: v})
	}
	return items
}

// repoItems offers the owners and the repositories of the threads cached
// for the inbox on view, the most recent first.
func (s *Section) repoItems() (owners, repos []filterform.Item) {
	seenOwner, seenRepo := map[string]bool{}, map[string]bool{}
	q := s.query("")
	for range maxRepoPages {
		p, ok := s.svc.CachedList(q)
		if !ok {
			break
		}
		for i := range p.Items {
			ref := p.Items[i].Repo
			if k := strings.ToLower(ref.Owner); !seenOwner[k] {
				seenOwner[k] = true
				owners = append(owners, filterform.Item{Label: ref.Owner, Value: k})
			}
			r := ref.String()
			if k := strings.ToLower(r); !seenRepo[k] {
				seenRepo[k] = true
				repos = append(repos, filterform.Item{Label: r, Value: k})
			}
		}
		if p.Last() {
			break
		}
		q.Cursor = p.Next
	}
	return owners, repos
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
	// The rows read ahead were the old filter's.
	s.opener.Stop()
	return s.feed.Reset()
}

// filtered reports whether the filter is other than the default inbox.
func (s *Section) filtered() bool { return s.filter().query != defaultQuery }

// chips names the filter in a few words, for the header, apart by sep.
func (f *filter) chips(sep string) string {
	parts := make([]string, 0, 3)
	for _, t := range filterform.Tokenize(f.query) {
		switch strings.ToLower(t.Qualifier) {
		case "is":
			switch strings.ToLower(t.Value) {
			case "unread":
			case "participating":
				parts = append(parts, "participating")
			default:
				parts = append(parts, t.Raw)
			}
		case "since":
			if v := strings.ToLower(t.Value); v == f.since {
				parts = append(parts, "since "+v)
			}
		case "reason":
			for _, v := range t.Values() {
				parts = append(parts, shortReason(strings.ToLower(v)))
			}
		case "owner", "repo", "type":
			parts = append(parts, t.Values()...)
		default:
			parts = append(parts, t.Raw)
		}
	}
	return strings.Join(parts, sep)
}

// listQuery is what GitHub is asked for the filter: read threads or not, the
// ones the user takes part in, and the ones updated within the window.
func (f *filter) listQuery(cursor string) notifications.ListQuery {
	return notifications.ListQuery{
		Filter: core.NotificationFilter{All: f.all, Participating: f.participating, Since: f.since},
		Cursor: cursor,
	}
}
