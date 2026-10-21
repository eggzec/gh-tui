package github

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// topicSearchResults is the body of a topic search response.
type topicSearchResults struct {
	TotalCount        int              `json:"total_count"`
	IncompleteResults bool             `json:"incomplete_results"`
	Items             []topicSearchHit `json:"items"`
}

// topicSearchHit is the REST shape of a topic in topic search results.
type topicSearchHit struct {
	Name string `json:"name"`
	// ShortDescription is null for a topic without one.
	ShortDescription *string `json:"short_description"`
}

func (h topicSearchHit) core() core.SearchTopic {
	t := core.SearchTopic{Name: h.Name}
	if h.ShortDescription != nil {
		t.ShortDescription = termtext.OneLine(*h.ShortDescription)
	}
	return t
}

// SearchTopics returns a page of the topics that match query, best match
// first. Query takes GitHub's topic search syntax, such as is:featured.
// Cursor and perPage work as in SearchRepos; topic searches share the search
// limit.
func (c *Client) SearchTopics(ctx context.Context, query, cursor string, perPage int) (core.SearchPage[core.SearchTopic], error) {
	var body topicSearchResults
	next, err := c.search(ctx, "search/topics", query, cursor, perPage, &body)
	if err != nil {
		return core.SearchPage[core.SearchTopic]{}, fmt.Errorf("search topics: %w", err)
	}
	return core.SearchPage[core.SearchTopic]{
		Items:      convert(body.Items, topicSearchHit.core),
		Next:       next,
		Total:      body.TotalCount,
		Incomplete: body.IncompleteResults,
	}, nil
}
