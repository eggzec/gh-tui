package github

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
)

// pullMergeTextQuery reads the commit message GitHub would use for a merge
// commit and for a squash commit of a pull request. The text is per viewer,
// since it names the viewer's own co-authors and settings.
var pullMergeTextQuery = `query PullMergeText($owner: String!, $name: String!, $number: Int!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      mergeHeadline: viewerMergeHeadlineText(mergeType: MERGE)
      mergeBody: viewerMergeBodyText(mergeType: MERGE)
      squashHeadline: viewerMergeHeadlineText(mergeType: SQUASH)
      squashBody: viewerMergeBodyText(mergeType: SQUASH)
    }
  }
}`

// PullMergeTexts returns the commit title and body that GitHub would use to
// merge pull request number of repo with a merge commit and with a squash.
// A rebase keeps each commit's own message, so it has none.
func (c *Client) PullMergeTexts(ctx context.Context, repo core.RepoRef, number int) (core.MergeTexts, error) {
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	var data struct {
		Repository *struct {
			PullRequest *struct {
				MergeHeadline  string `json:"mergeHeadline"`
				MergeBody      string `json:"mergeBody"`
				SquashHeadline string `json:"squashHeadline"`
				SquashBody     string `json:"squashBody"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	err := c.Query(ctx, pullMergeTextQuery, vars, &data)
	if err == nil && (data.Repository == nil || data.Repository.PullRequest == nil) {
		err = core.ErrNotFound
	}
	if err != nil {
		return core.MergeTexts{}, fmt.Errorf("read merge message of pull request %s#%d: %w", repo, number, err)
	}
	pr := data.Repository.PullRequest
	return core.MergeTexts{
		Merge:  core.MergeMessage{Title: pr.MergeHeadline, Body: pr.MergeBody, SendBody: true},
		Squash: core.MergeMessage{Title: pr.SquashHeadline, Body: pr.SquashBody, SendBody: true},
	}, nil
}
