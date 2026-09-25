package github

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Labels and milestones are read with REST, which answers a repeated read
// with a free 304 when nothing changed. Filters offer them, so one page of
// the most a repository is likely to have is enough.

// maxRepoLabels is how many labels and milestones a read returns, GitHub's
// largest page.
const maxRepoLabels = 100

// ListLabels returns the labels of repo, up to 100, by name.
func (c *Client) ListLabels(ctx context.Context, repo core.RepoRef, cond Conditional) ([]core.Label, Response, error) {
	var items []label
	res, err := c.Get(ctx, labelRepoPath(repo)+"/labels?per_page="+strconv.Itoa(maxRepoLabels), cond, &items)
	if err != nil || res.NotModified {
		return nil, res, err
	}
	return convert(items, label.core), res, nil
}

// restMilestone is the REST shape of a milestone.
type restMilestone struct {
	Number int        `json:"number"`
	Title  string     `json:"title"`
	State  string     `json:"state"`
	DueOn  *time.Time `json:"due_on"`
}

func (m restMilestone) core() core.Milestone {
	out := core.Milestone{Number: m.Number, Title: m.Title, State: core.State(m.State)}
	if m.DueOn != nil {
		out.DueOn = *m.DueOn
	}
	return out
}

// ListMilestones returns the open milestones of repo, up to 100, the one
// due soonest first.
func (c *Client) ListMilestones(ctx context.Context, repo core.RepoRef, cond Conditional) ([]core.Milestone, Response, error) {
	var items []restMilestone
	path := labelRepoPath(repo) + "/milestones?state=open&sort=due_on&direction=asc&per_page=" + strconv.Itoa(maxRepoLabels)
	res, err := c.Get(ctx, path, cond, &items)
	if err != nil || res.NotModified {
		return nil, res, err
	}
	return convert(items, restMilestone.core), res, nil
}

// labelRepoPath is the REST path of repo, under which its labels and
// milestones are.
func labelRepoPath(repo core.RepoRef) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name)
}
