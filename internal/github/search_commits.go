package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// commitSearchResults is the body of a commit search response.
type commitSearchResults struct {
	TotalCount        int               `json:"total_count"`
	IncompleteResults bool              `json:"incomplete_results"`
	Items             []commitSearchHit `json:"items"`
}

// commitSearchHit is the REST shape of a commit in commit search results.
type commitSearchHit struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Name string    `json:"name"`
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	// Author is null when the commit's author has no account.
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (h commitSearchHit) core() core.SearchCommit {
	owner, name, _ := strings.Cut(h.Repository.FullName, "/")
	subject, _, _ := strings.Cut(strings.TrimSpace(h.Commit.Message), "\n")
	author := h.Commit.Author.Name
	if h.Author != nil && h.Author.Login != "" {
		author = h.Author.Login
	}
	return core.SearchCommit{
		Repo:    repoRef(owner, name),
		SHA:     h.SHA,
		Subject: termtext.OneLine(subject),
		Author:  termtext.OneLine(author),
		Date:    h.Commit.Author.Date,
		URL:     h.HTMLURL,
	}
}

// SearchCommits returns a page of the commits on a default branch that match
// query, best match first. Query takes GitHub's commit search syntax, such as
// repo:, author: and org: qualifiers. Cursor and perPage work as in
// SearchRepos; commit searches share the search limit.
func (c *Client) SearchCommits(ctx context.Context, query, cursor string, perPage int) (core.SearchPage[core.SearchCommit], error) {
	var body commitSearchResults
	next, err := c.search(ctx, "search/commits", query, cursor, perPage, &body)
	if err != nil {
		return core.SearchPage[core.SearchCommit]{}, fmt.Errorf("search commits: %w", err)
	}
	return core.SearchPage[core.SearchCommit]{
		Items:      convert(body.Items, commitSearchHit.core),
		Next:       next,
		Total:      body.TotalCount,
		Incomplete: body.IncompleteResults,
	}, nil
}
