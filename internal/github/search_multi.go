package github

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Search runs one GraphQL query with a search field per kind. It costs a
// single point of the GraphQL quota, however many kinds it asks for, and
// leaves the REST search limit of 30 a minute alone, so a search page can
// count every kind as the user types.

// Limits of a search result row.
const (
	// searchLabels is how many labels a row shows.
	searchLabels = 5
	// searchPageSize is the page size of a SearchQuery that sets none.
	searchPageSize = 20
)

// searchVars names the variables of the search field of each kind in
// multiSearchQuery.
var searchVars = map[core.SearchKind]struct{ query, first, after, with string }{
	core.SearchRepos:  {"reposQuery", "reposFirst", "reposAfter", "withRepos"},
	core.SearchIssues: {"issuesQuery", "issuesFirst", "issuesAfter", "withIssues"},
	core.SearchPulls:  {"pullsQuery", "pullsFirst", "pullsAfter", "withPulls"},
}

// multiSearchQuery searches each kind that its with variable includes, for
// as many results as its first variable says; zero counts them alone.
// Issues and pull requests share the ISSUE type, so their queries add the
// qualifier that tells them apart.
var multiSearchQuery = fmt.Sprintf(`query Search($reposQuery: String!, $issuesQuery: String!, $pullsQuery: String!,
  $reposFirst: Int!, $issuesFirst: Int!, $pullsFirst: Int!,
  $reposAfter: String, $issuesAfter: String, $pullsAfter: String,
  $withRepos: Boolean!, $withIssues: Boolean!, $withPulls: Boolean!) {
  %s
  repos: search(type: REPOSITORY, query: $reposQuery, first: $reposFirst, after: $reposAfter) @include(if: $withRepos) {
    repositoryCount
    pageInfo { hasNextPage endCursor }
    nodes { ...searchRepo }
  }
  issues: search(type: ISSUE, query: $issuesQuery, first: $issuesFirst, after: $issuesAfter) @include(if: $withIssues) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes { ...searchIssue }
  }
  pulls: search(type: ISSUE, query: $pullsQuery, first: $pullsFirst, after: $pullsAfter) @include(if: $withPulls) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes { ...searchPull }
  }
}
fragment searchRepo on Repository {
  id
  name
  owner { login }
  description
  primaryLanguage { name }
  stargazerCount
  isPrivate
  isArchived
  isFork
  defaultBranchRef { name }
  updatedAt
  url
}
fragment searchIssue on Issue {
  __typename
  id
  number
  title
  state
  url
  createdAt
  updatedAt
  repository { name owner { login } }
  author { login ... on User { name } }
  labels(first: %[2]d) { nodes { name color description } }
  comments { totalCount }
}
fragment searchPull on PullRequest {
  __typename
  id
  number
  title
  state
  isDraft
  url
  createdAt
  updatedAt
  repository { name owner { login } }
  author { login ... on User { name } }
  labels(first: %[2]d) { nodes { name color description } }
  comments { totalCount }
}
`, rateLimitField, searchLabels)

// SearchQuery selects what Search looks for.
type SearchQuery struct {
	// Text is the query in GitHub's search syntax. Qualifiers such as
	// repo:, org: or language: apply to every kind they make sense for;
	// Search adds is:issue and is:pr itself.
	Text string
	// First is the page size of each kind, 1 to 100. Zero means 20.
	First int
	// After holds the kinds to search, core.SearchRepos, core.SearchIssues
	// or core.SearchPulls, each with the cursor of the page before the one
	// wanted: the Next of that kind's previous page, or empty for its first.
	// An empty After with no Count asks for the first page of every kind.
	After map[core.SearchKind]string
	// Count holds the kinds to count without their results, which GitHub
	// answers faster than a page. It must not share a kind with After.
	Count []core.SearchKind
}

// kindQuery returns the query text that searches for kind.
func kindQuery(text string, kind core.SearchKind) string {
	switch kind {
	case core.SearchIssues:
		return text + " is:issue"
	case core.SearchPulls:
		return text + " is:pr"
	default:
		return text
	}
}

func (q SearchQuery) vars() (map[string]any, error) {
	first := q.First
	if first == 0 {
		first = searchPageSize
	}
	if first < 1 || first > 100 {
		return nil, fmt.Errorf("page size %d is not between 1 and 100", first)
	}
	after := q.After
	if len(after) == 0 && len(q.Count) == 0 {
		after = map[core.SearchKind]string{core.SearchRepos: "", core.SearchIssues: "", core.SearchPulls: ""}
	}
	vars := make(map[string]any, 4*len(searchVars))
	for kind, v := range searchVars {
		cursor, ok := after[kind]
		counted := slices.Contains(q.Count, kind)
		if ok && counted {
			return nil, fmt.Errorf("cannot both list and count %q", kind)
		}
		vars[v.query] = kindQuery(q.Text, kind)
		vars[v.with] = ok || counted
		vars[v.first] = first
		if counted {
			vars[v.first] = 0
		}
		if cursor != "" {
			vars[v.after] = cursor
		}
	}
	for kind := range after {
		if _, ok := searchVars[kind]; !ok {
			return nil, fmt.Errorf("cannot search for %q", kind)
		}
	}
	for _, kind := range q.Count {
		if _, ok := searchVars[kind]; !ok {
			return nil, fmt.Errorf("cannot count %q", kind)
		}
	}
	return vars, nil
}

// searchRepoNode is the JSON shape of the searchRepo fragment.
type searchRepoNode struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Description     string `json:"description"`
	PrimaryLanguage *struct {
		Name string `json:"name"`
	} `json:"primaryLanguage"`
	StargazerCount   int  `json:"stargazerCount"`
	IsPrivate        bool `json:"isPrivate"`
	IsArchived       bool `json:"isArchived"`
	IsFork           bool `json:"isFork"`
	DefaultBranchRef *struct {
		Name string `json:"name"`
	} `json:"defaultBranchRef"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url"`
}

// hit maps the repository. Search results don't say whether the viewer
// starred it, so Starred is false.
func (r searchRepoNode) hit() core.SearchHit {
	repo := core.Repo{
		ID:          r.ID,
		Ref:         core.RepoRef{Owner: r.Owner.Login, Name: r.Name},
		Description: r.Description,
		Stars:       r.StargazerCount,
		Private:     r.IsPrivate,
		Fork:        r.IsFork,
		Archived:    r.IsArchived,
		UpdatedAt:   r.UpdatedAt,
		URL:         r.URL,
	}
	if r.PrimaryLanguage != nil {
		repo.Language = r.PrimaryLanguage.Name
	}
	// An empty repository has no default branch.
	if r.DefaultBranchRef != nil {
		repo.DefaultBranch = r.DefaultBranchRef.Name
	}
	return core.SearchHit{Kind: core.SearchRepos, Repo: repo}
}

// searchIssueNode is the JSON shape of the searchIssue and searchPull
// fragments.
type searchIssueNode struct {
	Typename   string    `json:"__typename"`
	ID         string    `json:"id"`
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	IsDraft    bool      `json:"isDraft"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	// Author is null for deleted accounts.
	Author   *user        `json:"author"`
	Labels   nodes[label] `json:"labels"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
}

// hit maps the issue or pull request. A pull request's state may be
// merged, and Draft is set only on the hit, as core.Issue has no draft.
func (n searchIssueNode) hit() core.SearchHit {
	is := core.Issue{
		ID:        n.ID,
		Repo:      core.RepoRef{Owner: n.Repository.Owner.Login, Name: n.Repository.Name},
		Number:    n.Number,
		Title:     n.Title,
		State:     core.State(strings.ToLower(n.State)),
		Labels:    convert(n.Labels.Nodes, label.core),
		Comments:  n.Comments.TotalCount,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
		URL:       n.URL,
	}
	if n.Author != nil {
		is.Author = n.Author.core()
	}
	hit := core.SearchHit{Kind: core.SearchIssues, Issue: is}
	if n.Typename == "PullRequest" {
		hit.Kind = core.SearchPulls
		hit.Draft = n.IsDraft
	}
	return hit
}

// searchConn is the JSON shape of one search field. RepositoryCount and
// IssueCount are the total of their type; the other is zero.
type searchConn[T any] struct {
	RepositoryCount int      `json:"repositoryCount"`
	IssueCount      int      `json:"issueCount"`
	PageInfo        pageInfo `json:"pageInfo"`
	// Nodes can hold nulls, for results the viewer may not see.
	Nodes []*T `json:"nodes"`
}

func (s *searchConn[T]) page(hit func(T) core.SearchHit) core.SearchPage[core.SearchHit] {
	hits := make([]core.SearchHit, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		if n != nil {
			hits = append(hits, hit(*n))
		}
	}
	return core.SearchPage[core.SearchHit]{
		Items: hits, Next: s.PageInfo.next(),
		Total: s.RepositoryCount + s.IssueCount,
	}
}

// searchData is the data of multiSearchQuery. A kind left out of the query
// is nil.
type searchData struct {
	Repos  *searchConn[searchRepoNode]  `json:"repos"`
	Issues *searchConn[searchIssueNode] `json:"issues"`
	Pulls  *searchConn[searchIssueNode] `json:"pulls"`
}

func (d *searchData) pages() map[core.SearchKind]core.SearchPage[core.SearchHit] {
	out := make(map[core.SearchKind]core.SearchPage[core.SearchHit], len(searchVars))
	if d.Repos != nil {
		out[core.SearchRepos] = d.Repos.page(searchRepoNode.hit)
	}
	if d.Issues != nil {
		out[core.SearchIssues] = d.Issues.page(searchIssueNode.hit)
	}
	if d.Pulls != nil {
		out[core.SearchPulls] = d.Pulls.page(searchIssueNode.hit)
	}
	return out
}

// Search returns a page of repositories, issues and pull requests that
// match q.Text, with the number of matches of each kind, best match first.
// It asks for the kinds in q.After and q.Count, or for all three, in one
// GraphQL query: paging through one kind asks for that kind alone. The
// result holds a page for each kind asked for; a kind only counted has its
// Total and no results.
func (c *Client) Search(ctx context.Context, q SearchQuery) (map[core.SearchKind]core.SearchPage[core.SearchHit], error) {
	vars, err := q.vars()
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", q.Text, err)
	}
	var data searchData
	if err := c.Query(ctx, multiSearchQuery, vars, &data); err != nil {
		return nil, fmt.Errorf("search %q: %w", q.Text, err)
	}
	return data.pages(), nil
}
