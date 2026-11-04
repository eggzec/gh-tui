package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// issueDependency is an issue of the dependencies endpoints.
type issueDependency struct {
	Number        int            `json:"number"`
	Title         string         `json:"title"`
	State         core.State     `json:"state"`
	StateReason   string         `json:"state_reason"`
	HTMLURL       string         `json:"html_url"`
	RepositoryURL string         `json:"repository_url"`
	PullRequest   map[string]any `json:"pull_request"`
}

func (d issueDependency) core() core.Dependency {
	out := core.Dependency{
		Number: d.Number, Title: d.Title, State: d.State,
		Reason: core.StateReason(d.StateReason), URL: d.HTMLURL, IsPullRequest: d.PullRequest != nil,
	}
	// repository_url ends in repos/{owner}/{name}, so a blocker in another
	// repository keeps its own.
	if parts := strings.Split(strings.TrimRight(d.RepositoryURL, "/"), "/"); len(parts) >= 2 {
		out.Repo = core.RepoRef{Owner: parts[len(parts)-2], Name: parts[len(parts)-1]}
	}
	return out
}

// ListIssueDependencies returns the issues that block an issue (kind
// "blocked_by") or that it blocks (kind "blocking"). It reads the first page
// only, which holds up to 100. A repository without the feature answers 404,
// which comes back as core.ErrNotFound.
func (c *Client) ListIssueDependencies(ctx context.Context, repo core.RepoRef, number int, kind string) ([]core.Dependency, error) {
	var items []issueDependency
	if _, err := c.Get(ctx, issuePath(repo, number)+"/dependencies/"+kind+"?per_page=100", Conditional{}, &items); err != nil {
		return nil, fmt.Errorf("list %s of issue %s#%d: %w", kind, repo, number, err)
	}
	return convert(items, issueDependency.core), nil
}
