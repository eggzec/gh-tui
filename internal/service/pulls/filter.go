package pulls

import (
	"context"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/terms"
)

// readList reads the page of q from the repository's list when it can
// select what the filter asks for, and with a search otherwise.
func (s *Service) readList(ctx context.Context, q ListQuery) (core.Page[core.PullRequest], error) {
	size := pageSize(q.PageSize, s.pageSize)
	if q.Filter == "" {
		return s.api.ListPullRequests(ctx, q.Repo, q.State, q.Cursor, size)
	}
	if f, ok := listFilter(q); ok {
		return s.api.FilterPullRequests(ctx, q.Repo, f, q.Cursor, size)
	}
	return s.api.SearchPullRequests(ctx, searchQuery(q), q.Cursor, size)
}

// listFilter returns the filter of the repository's list that selects what
// q does, and reports false if the list can't. The list takes any of its
// labels, as label:a,b does, so two label: qualifiers, which want both,
// take a search.
func listFilter(q ListQuery) (github.PullFilter, bool) {
	f := github.PullFilter{State: q.State}
	labels := false
	for _, t := range terms.Parse(q.Filter) {
		if t.Not {
			return f, false
		}
		switch t.Key {
		case "label":
			if labels {
				return f, false
			}
			f.Labels, labels = t.Values(), true
		case "base":
			if f.Base != "" {
				return f, false
			}
			f.Base = t.Value
		case "head":
			if f.Head != "" {
				return f, false
			}
			f.Head = t.Value
		case "sort":
			by, asc := terms.Sort(t.Value)
			if by != "updated" && by != "created" && by != "comments" {
				return f, false
			}
			f.Sort, f.Asc = by, asc
		default:
			return f, false
		}
	}
	return f, true
}

// searchQuery returns the search that finds the pull requests of q. A
// search orders by best match unless told otherwise, so it gets the list's
// order.
func searchQuery(q ListQuery) string {
	parts := []string{"repo:" + q.Repo.String(), "is:pr"}
	switch q.State {
	case core.StateOpen:
		parts = append(parts, "is:open")
	case core.StateClosed:
		// The list's closed pull requests are those closed unmerged.
		parts = append(parts, "is:closed", "is:unmerged")
	case core.StateMerged:
		parts = append(parts, "is:merged")
	default:
	}
	parts = append(parts, q.Filter)
	if !sorted(q.Filter) {
		parts = append(parts, "sort:updated-desc")
	}
	return strings.Join(parts, " ")
}

// sorted reports whether filter has a sort: qualifier.
func sorted(filter string) bool {
	for _, t := range terms.Parse(filter) {
		if t.Key == "sort" && !t.Not {
			return true
		}
	}
	return false
}
