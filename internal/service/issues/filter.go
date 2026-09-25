package issues

import (
	"context"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/terms"
)

// readList reads the page of q from the repository's list when it can
// select what the filter asks for, and with a search otherwise.
func (s *Service) readList(ctx context.Context, q ListQuery, cond github.Conditional) (core.Page[core.Issue], github.Response, error) {
	if q.Filter == "" {
		return s.api.ListIssues(ctx, q.Repo, q.State, q.Cursor, q.PageSize, cond)
	}
	if f, ok := s.listFilter(ctx, q); ok {
		return s.api.FilterIssues(ctx, q.Repo, f, q.Cursor, q.PageSize, cond)
	}
	hits, err := s.api.SearchIssues(ctx, searchQuery(q), q.Cursor, q.PageSize)
	if err != nil {
		return core.Page[core.Issue]{}, github.Response{}, err
	}
	page := core.Page[core.Issue]{Next: hits.Next}
	for i := range hits.Items {
		if hits.Items[i].Kind == core.SearchIssues {
			page.Items = append(page.Items, hits.Items[i].Issue)
		}
	}
	return page, github.Response{}, nil
}

// listFilter returns the filter of the repository's list that selects what
// q does, and reports false if the list can't. The list wants every label
// it is given, as label:a label:b does, so label:a,b, which wants any of
// them, takes a search; so does a label with a comma, which the list would
// split.
func (s *Service) listFilter(ctx context.Context, q ListQuery) (github.IssueFilter, bool) {
	f := github.IssueFilter{State: q.State}
	for _, t := range terms.Parse(q.Filter) {
		if t.Not {
			return f, false
		}
		var ok bool
		switch t.Key {
		case "label":
			vs := t.Values()
			ok = len(vs) == 1 && !strings.Contains(vs[0], ",")
			f.Labels = append(f.Labels, vs...)
		case "assignee":
			ok = setOnce(&f.Assignee, s.login(ctx, t.Value))
		case "author":
			ok = setOnce(&f.Creator, s.login(ctx, t.Value))
		case "mentions":
			ok = setOnce(&f.Mentioned, s.login(ctx, t.Value))
		case "no":
			switch strings.ToLower(t.Value) {
			case "assignee":
				ok = setOnce(&f.Assignee, "none")
			case "milestone":
				ok = setOnce(&f.Milestone, "none")
			default:
				ok = false
			}
		case "sort":
			by, asc := terms.Sort(t.Value)
			ok = by == "updated" || by == "created" || by == "comments"
			f.Sort, f.Asc = by, asc
		default:
			ok = false
		}
		if !ok {
			return f, false
		}
	}
	return f, true
}

// setOnce sets *dst to v, and reports false if v is empty, as a login that
// can't be told is, or *dst was set already, which the list can't take.
func setOnce(dst *string, v string) bool {
	if v == "" || *dst != "" {
		return false
	}
	*dst = v
	return true
}

// login returns the login a qualifier names, with @me read as the
// signed-in user's, or "" if that can't be read.
func (s *Service) login(ctx context.Context, v string) string {
	if !strings.EqualFold(v, "@me") {
		return strings.TrimPrefix(v, "@")
	}
	if s.viewer != "" {
		return s.viewer
	}
	s.meMu.Lock()
	defer s.meMu.Unlock()
	if s.me == "" {
		// A failure leaves the list to a search, which knows @me, and
		// the next filter asks again.
		s.me, _ = s.api.ViewerLogin(ctx)
	}
	return s.me
}

// searchQuery returns the search that finds the issues of q. A search
// orders by best match unless told otherwise, so it gets the list's order.
func searchQuery(q ListQuery) string {
	parts := []string{"repo:" + q.Repo.String(), "is:issue"}
	switch q.State {
	case core.FilterOpen:
		parts = append(parts, "is:open")
	case core.FilterClosed:
		parts = append(parts, "is:closed")
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
