package github

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// SearchRepos and SearchIssues use REST, which pages with Link headers like
// the other lists and returns issues in the shape the issue methods already
// decode. Search, in search_multi.go, asks for every kind at once instead. Search has
// its own rate limit of 30 requests a minute; running out of it fails with a
// *core.RateLimitError like any other limit.

// searchRepo is the REST shape of a repository in search results.
type searchRepo struct {
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
	Owner  struct {
		Login string `json:"login"`
	} `json:"owner"`
	Description   string    `json:"description"`
	DefaultBranch string    `json:"default_branch"`
	Language      string    `json:"language"`
	Stars         int       `json:"stargazers_count"`
	Private       bool      `json:"private"`
	Fork          bool      `json:"fork"`
	Archived      bool      `json:"archived"`
	Template      bool      `json:"is_template"`
	MirrorURL     string    `json:"mirror_url"`
	UpdatedAt     time.Time `json:"updated_at"`
	HTMLURL       string    `json:"html_url"`
}

// core maps the repository. Search results don't say whether the viewer
// starred it, so Starred is false.
func (r searchRepo) core() core.Repo {
	return core.Repo{
		ID:            r.NodeID,
		Ref:           repoRef(r.Owner.Login, r.Name),
		Description:   r.Description,
		DefaultBranch: r.DefaultBranch,
		Language:      r.Language,
		Stars:         r.Stars,
		Private:       r.Private,
		Fork:          r.Fork,
		Archived:      r.Archived,
		Template:      r.Template,
		Mirror:        r.MirrorURL != "",
		UpdatedAt:     r.UpdatedAt,
		URL:           r.HTMLURL,
	}
}

// searchIssue is the REST shape of an issue or pull request in search
// results. Unlike a repository's issue list, it names its repository.
type searchIssue struct {
	restIssue
	RepositoryURL string `json:"repository_url"`
	// PullRequest shadows the one in restIssue to read when a pull request
	// was merged, which search reports only as closed.
	PullRequest *struct {
		MergedAt *time.Time `json:"merged_at"`
	} `json:"pull_request"`
}

func (i searchIssue) core() core.SearchHit {
	hit := core.SearchHit{Kind: core.SearchIssues, Issue: i.restIssue.core(repoFromURL(i.RepositoryURL))}
	if i.PullRequest != nil {
		hit.Kind = core.SearchPulls
		if i.PullRequest.MergedAt != nil {
			hit.Issue.State = core.StateMerged
		}
	}
	return hit
}

// repoFromURL reads the owner and name from a repository's API URL, such as
// https://api.github.com/repos/octo-org/hello.
func repoFromURL(s string) core.RepoRef {
	u, err := url.Parse(s)
	if err != nil {
		return core.RepoRef{}
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return core.RepoRef{}
	}
	return repoRef(parts[len(parts)-2], parts[len(parts)-1])
}

// searchResults is the body of a search response.
type searchResults[T any] struct {
	Items []T `json:"items"`
}

// SearchRepos returns a page of the repositories that match query, best
// match first. Query takes GitHub's search syntax, qualifiers included.
// Cursor is the Next of the previous page, or empty for the first. PerPage
// sizes the first page, and 0 leaves the size to GitHub; later pages keep
// the size of the page their cursor came from.
func (c *Client) SearchRepos(ctx context.Context, query, cursor string, perPage int) (core.Page[core.Repo], error) {
	var body searchResults[searchRepo]
	next, err := c.search(ctx, "search/repositories", query, cursor, perPage, &body)
	if err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("search repos: %w", err)
	}
	return core.Page[core.Repo]{Items: convert(body.Items, searchRepo.core), Next: next}, nil
}

// SearchIssues returns a page of the issues and pull requests that match
// query, best match first, as hits of kind core.SearchIssues or
// core.SearchPulls. Add is:issue or is:pr to query for one kind only.
// Cursor and perPage work as in SearchRepos.
func (c *Client) SearchIssues(ctx context.Context, query, cursor string, perPage int) (core.Page[core.SearchHit], error) {
	var body searchResults[searchIssue]
	next, err := c.search(ctx, "search/issues", query, cursor, perPage, &body)
	if err != nil {
		return core.Page[core.SearchHit]{}, fmt.Errorf("search issues: %w", err)
	}
	return core.Page[core.SearchHit]{Items: convert(body.Items, searchIssue.core), Next: next}, nil
}

// search reads one page of a search endpoint into v and returns the cursor
// of the next page.
func (c *Client) search(ctx context.Context, endpoint, query, cursor string, perPage int, v any) (string, error) {
	path := cursor
	if path == "" {
		q := url.Values{"q": {query}}
		if perPage > 0 {
			q.Set("per_page", strconv.Itoa(perPage))
		}
		path = endpoint + "?" + q.Encode()
	}
	res, err := c.Get(ctx, path, Conditional{}, v)
	if err != nil {
		return "", err
	}
	return res.Next, nil
}
