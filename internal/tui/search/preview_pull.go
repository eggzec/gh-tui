package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// PullPreviews reads what the preview of a pull request shows beyond its
// hit: its detail and its commits.
type PullPreviews interface {
	Get(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	PullCommits(ctx context.Context, q pulls.PullCommitsQuery) (core.PullCommits, error)
}

// pullReader returns the read of a pull request hit's preview.
func pullReader(pp PullPreviews, ellipsis string, h core.SearchHit) func(ctx context.Context) (string, error) {
	if pp == nil {
		return nil
	}
	repo, number := h.Issue.Repo, h.Issue.Number
	return func(ctx context.Context) (string, error) {
		d, err := pp.Get(ctx, repo, number)
		if err != nil {
			return "", err
		}
		c, err := pp.PullCommits(ctx, pulls.PullCommitsQuery{Repo: repo, Number: number, Head: d.HeadSHA})
		if err != nil {
			return "", err
		}
		return pullExtra(d, c, ellipsis), nil
	}
}

// pullExtra is what a read of a pull request adds under the hit's own text,
// as Markdown: the branches, the line counts, the review status and who was
// asked, and the commits.
func pullExtra(d core.PullRequestDetail, c core.PullCommits, ellipsis string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Branch: `%s` -> `%s`\n", d.HeadRef, d.BaseRef)
	fmt.Fprintf(&b, "- Changes: +%d -%d in %d files\n", d.Additions, d.Deletions, d.ChangedFiles)
	if !d.UpdatedAt.IsZero() {
		fmt.Fprintf(&b, "- Updated: %s\n", d.UpdatedAt.Format("2006-01-02 15:04"))
	}
	status := string(d.ReviewDecision)
	if status == "" {
		status = "none"
	}
	fmt.Fprintf(&b, "- Review status: %s\n", status)
	var who []string
	for _, v := range d.Reviewers.Latest {
		who = append(who, fmt.Sprintf("%s (%s)", v.Author.Login, v.State))
	}
	for _, r := range d.Reviewers.Requested {
		switch {
		case r.User.Login != "":
			who = append(who, r.User.Login+" (requested)")
		case r.Team != "":
			who = append(who, r.Team+" (requested)")
		}
	}
	if len(who) > 0 {
		fmt.Fprintf(&b, "- Reviewers: %s\n", strings.Join(who, ", "))
	}
	if len(c.Commits) > 0 {
		b.WriteString("\nCommits:\n\n")
		for _, k := range c.Commits {
			oid := k.OID
			if len(oid) > 7 {
				oid = oid[:7]
			}
			fmt.Fprintf(&b, "- `%s` %s\n", oid, strings.ReplaceAll(k.Headline, "\n", " "))
		}
		if c.Capped {
			fmt.Fprintf(&b, "- %s\n", ellipsis)
		}
	}
	return b.String()
}
