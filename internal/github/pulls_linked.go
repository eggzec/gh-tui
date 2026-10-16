package github

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// The most that one read of the links of a pull request asks for. A total
// past what came is told as "+N more", never paged.
const (
	linkedIssuesFirst   = 25
	linkedBlockersFirst = 10
	linkedPullsFirst    = 10
	linkedLabelsFirst   = 20
	linkedCommentsLast  = 3
	linkedReviewsFirst  = 10
)

// linkedIssueFragment and linkedPullFragment select what names an issue or a
// pull request in a list. GitHub refuses a fragment that nothing spreads, so
// a query appends only those it uses.
const (
	linkedIssueFragment = `
fragment linkedIssue on Issue { id number title state stateReason url repository { name owner { login } } }`
	linkedPullFragment = `
fragment linkedPull on PullRequest { id number title state isDraft url repository { name owner { login } } }`
)

// pullLinksQuery reads the issues a pull request closes, what blocks each of
// them one level deep, and the pull requests that close them too. Issue.blockedBy
// is not in the schema of an Enterprise Server up to 3.16, so a read of such a
// server sends it without that field.
var pullLinksQuery = fmt.Sprintf(`query PullLinks($owner: String!, $name: String!, $number: Int!) {
  `+rateLimitField+`
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      id
      closingIssuesReferences(first: %d) {
        totalCount
        nodes {
          ...linkedIssue
          blockedBy(first: %d) { totalCount nodes { ...linkedIssue } }
          closedByPullRequestsReferences(first: %d, includeClosedPrs: true) { totalCount nodes { ...linkedPull } }
        }
      }
    }
  }
}`, linkedIssuesFirst, linkedBlockersFirst, linkedPullsFirst) + linkedIssueFragment + linkedPullFragment

// issuePreviewQuery reads the short preview of an issue.
var issuePreviewQuery = fmt.Sprintf(`query IssuePreview($owner: String!, $name: String!, $number: Int!) {
  `+rateLimitField+`
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      ...linkedIssue
      author { login }
      createdAt
      body
      labels(first: %d) { nodes { name color } }
      blockedBy(first: %d) { totalCount nodes { ...linkedIssue } }
      comments(last: %d) { totalCount nodes { author { login } body createdAt } }
    }
  }
}`, linkedLabelsFirst, linkedBlockersFirst, linkedCommentsLast) + linkedIssueFragment

// pullPreviewQuery reads the short preview of a pull request, with the
// counts of the checks of its head commit.
var pullPreviewQuery = fmt.Sprintf(`query PullPreview($owner: String!, $name: String!, $number: Int!) {
  `+rateLimitField+`
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      ...linkedPull
      author { login }
      headRefName
      baseRefName
      additions
      deletions
      changedFiles
      reviewDecision
      body
      labels(first: %d) { nodes { name color } }
      latestOpinionatedReviews(first: %d) { nodes { author { login } state } }
      statusCheckRollup {
        state
        contexts(first: 1) {
          totalCount
          checkRunCountsByState { count state }
          statusContextCountsByState { count state }
        }
      }
    }
  }
}`, linkedLabelsFirst, linkedReviewsFirst) + linkedPullFragment

// linkedNode is an issue or a pull request as the linked fragments read it.
type linkedNode struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`
	StateReason string `json:"stateReason"`
	IsDraft     bool   `json:"isDraft"`
	URL         string `json:"url"`
	Repository  struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

// linkedRefused is why a node that GitHub left null can't be read.
const linkedRefused = "not found, or private"

// core returns the node as an item of kind. A node that is null, because
// GitHub refused it, has none of the fields: it is a row that says why.
func (n *linkedNode) core(kind core.NumberKind) core.Reference {
	if n == nil || n.Number == 0 {
		return core.Reference{Problem: linkedRefused}
	}
	return core.Reference{
		Target: core.Target{
			Repo:   repoRef(n.Repository.Owner.Login, n.Repository.Name),
			Number: n.Number,
			Kind:   kind,
		},
		Title: oneLine(n.Title), State: refStates[n.State], Draft: n.IsDraft,
		Reason: refReasons[n.StateReason], URL: n.URL,
	}
}

// linkedList is a connection of linked nodes with its total.
type linkedList struct {
	TotalCount int           `json:"totalCount"`
	Nodes      []*linkedNode `json:"nodes"`
}

// refs returns the items of the list, of kind.
func (l linkedList) refs(kind core.NumberKind) []core.Reference {
	out := make([]core.Reference, 0, len(l.Nodes))
	for _, n := range l.Nodes {
		out = append(out, n.core(kind))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// linkedIssueNode is a closing issue of a pull request.
type linkedIssueNode struct {
	linkedNode
	BlockedBy linkedList `json:"blockedBy"`
	ClosedBy  linkedList `json:"closedByPullRequestsReferences"`
}

// linkedPullNode is the pull request of pullLinksQuery: its node ID, to leave
// it out of the pull requests that close the same issues, and its closing
// issues.
type linkedPullNode struct {
	ID     string `json:"id"`
	Closes struct {
		TotalCount int                `json:"totalCount"`
		Nodes      []*linkedIssueNode `json:"nodes"`
	} `json:"closingIssuesReferences"`
}

// PullLinks reads the issues that pull request number of repo closes, the
// issues that block each, and the other pull requests that close the same
// issues. A node GitHub refuses, such as an issue of an organization that
// enforces SAML, comes as a row with Problem set, and the rest is kept. The
// errors name no read, since the service that calls this does.
func (c *Client) PullLinks(ctx context.Context, repo core.RepoRef, number int) (core.PullLinks, error) {
	var data struct {
		Repository *struct {
			PullRequest *linkedPullNode `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	err := c.Query(ctx, pullLinksQuery, vars, &data)
	var pr *linkedPullNode
	if data.Repository != nil {
		pr = data.Repository.PullRequest
	}
	if err != nil && (pr == nil || !refPartial(err)) {
		return core.PullLinks{}, err
	}
	if pr == nil {
		return core.PullLinks{}, core.ErrNotFound
	}
	supported := c.blockersSupported(ctx, pullLinksQuery)
	out := core.PullLinks{ClosesTotal: pr.Closes.TotalCount, BlockersSupported: supported}
	seen := map[string]bool{pr.ID: true}
	for _, n := range pr.Closes.Nodes {
		if n == nil || n.Number == 0 {
			out.Closes = append(out.Closes, core.LinkedIssue{Issue: (*linkedNode)(nil).core(core.KindIssue)})
			continue
		}
		li := core.LinkedIssue{Issue: n.core(core.KindIssue)}
		if supported {
			li.Blockers, li.BlockersTotal = n.BlockedBy.refs(core.KindIssue), n.BlockedBy.TotalCount
		}
		out.Closes = append(out.Closes, li)
		if n.ClosedBy.TotalCount > len(n.ClosedBy.Nodes) {
			out.LinkedCapped = true
		}
		for _, p := range n.ClosedBy.Nodes {
			if p == nil || p.Number == 0 {
				out.Linked = append(out.Linked, (*linkedNode)(nil).core(core.KindPull))
				continue
			}
			if seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			out.Linked = append(out.Linked, p.core(core.KindPull))
		}
	}
	// This repository's pull requests come first, then those of other
	// repositories by name, and the rows that say why a node can't be read last.
	rank := func(r core.Reference) int {
		switch {
		case r.Problem != "":
			return 2
		case r.Target.Repo == repo:
			return 0
		}
		return 1
	}
	slices.SortStableFunc(out.Linked, func(a, b core.Reference) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(a.Target.Repo.String(), b.Target.Repo.String()),
			cmp.Compare(a.Target.Number, b.Target.Number))
	})
	return out, nil
}

// blockersSupported reports whether the server has issue dependencies:
// github.com always does, and an Enterprise Server unless it said that its
// schema lacks blockedBy, so the query was sent without it. The first read
// that finds it so logs it, and the rest don't.
func (c *Client) blockersSupported(ctx context.Context, query string) bool {
	if strings.Contains(c.unsupported.rewrite(query), "blockedBy") {
		return true
	}
	if c.unsupported.first("blockedBy") {
		slog.WarnContext(ctx, "issue dependencies unsupported, blockers left out", "span", "github.linked",
			"ghes_version", c.enterpriseVersion())
	}
	return false
}

// linkedLabels are the labels of a preview.
type linkedLabels struct {
	Nodes []label `json:"nodes"`
}

// issuePreviewNode is the issue of issuePreviewQuery.
type issuePreviewNode struct {
	linkedNode
	Author    *actor       `json:"author"`
	CreatedAt time.Time    `json:"createdAt"`
	Body      string       `json:"body"`
	Labels    linkedLabels `json:"labels"`
	BlockedBy linkedList   `json:"blockedBy"`
	Comments  struct {
		TotalCount int `json:"totalCount"`
		Nodes      []*struct {
			Author    *actor    `json:"author"`
			Body      string    `json:"body"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"nodes"`
	} `json:"comments"`
}

// IssuePreview reads the short preview of issue number of repo. A blocker
// that GitHub refuses comes as a row with Problem set, and the rest is kept.
// The errors name no read, since the service that calls this does.
func (c *Client) IssuePreview(ctx context.Context, repo core.RepoRef, number int) (core.IssuePreview, error) {
	var data struct {
		Repository *struct {
			Issue *issuePreviewNode `json:"issue"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	err := c.Query(ctx, issuePreviewQuery, vars, &data)
	var i *issuePreviewNode
	if data.Repository != nil {
		i = data.Repository.Issue
	}
	if err != nil && (i == nil || !refPartial(err)) {
		return core.IssuePreview{}, err
	}
	if i == nil {
		return core.IssuePreview{}, core.ErrNotFound
	}
	out := core.IssuePreview{
		Reference: i.core(core.KindIssue), CreatedAt: i.CreatedAt, Body: i.Body,
		Labels:            convert(i.Labels.Nodes, label.core),
		BlockersSupported: c.blockersSupported(ctx, issuePreviewQuery),
		CommentsTotal:     i.Comments.TotalCount,
	}
	if i.Author != nil {
		out.Author = i.Author.Login
	}
	if out.BlockersSupported {
		out.Blockers, out.BlockersTotal = i.BlockedBy.refs(core.KindIssue), i.BlockedBy.TotalCount
	}
	for _, cm := range i.Comments.Nodes {
		if cm == nil {
			continue
		}
		pc := core.PreviewComment{Body: cm.Body, CreatedAt: cm.CreatedAt}
		if cm.Author != nil {
			pc.Author = cm.Author.Login
		}
		out.Comments = append(out.Comments, pc)
	}
	return out, nil
}

// pullPreviewNode is the pull request of pullPreviewQuery.
type pullPreviewNode struct {
	linkedNode
	Author         *actor       `json:"author"`
	HeadRefName    string       `json:"headRefName"`
	BaseRefName    string       `json:"baseRefName"`
	Additions      int          `json:"additions"`
	Deletions      int          `json:"deletions"`
	ChangedFiles   int          `json:"changedFiles"`
	ReviewDecision string       `json:"reviewDecision"`
	Body           string       `json:"body"`
	Labels         linkedLabels `json:"labels"`
	Reviews        struct {
		Nodes []*struct {
			Author *actor `json:"author"`
			State  string `json:"state"`
		} `json:"nodes"`
	} `json:"latestOpinionatedReviews"`
	Rollup *struct {
		Contexts pullCheckCounts `json:"contexts"`
	} `json:"statusCheckRollup"`
}

// PullPreview reads the short preview of pull request number of repo, with
// the counts of the checks of its head commit. A part that GitHub refuses is
// left out and the rest is kept. The errors name no read, since the service
// that calls this does.
func (c *Client) PullPreview(ctx context.Context, repo core.RepoRef, number int) (core.PullPreview, error) {
	var data struct {
		Repository *struct {
			PullRequest *pullPreviewNode `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	err := c.Query(ctx, pullPreviewQuery, vars, &data)
	var p *pullPreviewNode
	if data.Repository != nil {
		p = data.Repository.PullRequest
	}
	if err != nil && (p == nil || !refPartial(err)) {
		return core.PullPreview{}, err
	}
	if p == nil {
		return core.PullPreview{}, core.ErrNotFound
	}
	out := core.PullPreview{
		Reference: p.core(core.KindPull), Head: p.HeadRefName, Base: p.BaseRefName,
		Additions: p.Additions, Deletions: p.Deletions, ChangedFiles: p.ChangedFiles,
		ReviewDecision: core.ReviewDecision(strings.ToLower(p.ReviewDecision)),
		Labels:         convert(p.Labels.Nodes, label.core), Body: p.Body,
	}
	if p.Author != nil {
		out.Author = p.Author.Login
	}
	if p.Rollup != nil {
		out.Checks = p.Rollup.Contexts.core()
	}
	for _, r := range p.Reviews.Nodes {
		if r == nil {
			continue
		}
		pr := core.PreviewReview{State: core.ReviewState(strings.ToLower(r.State))}
		if r.Author != nil {
			pr.Author = r.Author.Login
		}
		out.Reviews = append(out.Reviews, pr)
	}
	return out, nil
}
