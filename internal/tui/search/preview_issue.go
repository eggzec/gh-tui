package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/refs"
)

// IssuePreviews reads the pull requests that refer to an issue.
type IssuePreviews interface {
	References(ctx context.Context, q refs.Query) (core.References, error)
	Mentions(ctx context.Context, q refs.MentionsQuery) (core.Page[core.Reference], error)
}

// issueReader returns the read of an issue hit's preview: the pull requests
// that close it, link it in their text or mention it.
func issueReader(ip IssuePreviews, h core.SearchHit) func(ctx context.Context) (string, error) {
	if ip == nil {
		return nil
	}
	repo, number := h.Issue.Repo, h.Issue.Number
	return func(ctx context.Context) (string, error) {
		r, err := ip.References(ctx, refs.Query{Repo: repo, Number: number, Updated: h.Issue.UpdatedAt})
		if err != nil {
			return "", err
		}
		m, err := ip.Mentions(ctx, refs.MentionsQuery{Repo: repo, Number: number})
		if err != nil {
			return "", err
		}
		return issueExtra(repo, r, m.Items), nil
	}
}

// issueExtra lists, as Markdown, the pull requests among the references of
// an issue, each once, the strongest link first.
func issueExtra(repo core.RepoRef, r core.References, mentions []core.Reference) string {
	all := make([]core.Reference, 0, len(r.Closing)+len(r.Written)+len(mentions))
	all = append(all, r.Closing...)
	all = append(all, r.Written...)
	all = append(all, mentions...)
	seen := map[string]bool{}
	var b strings.Builder
	for i := range all {
		ref := &all[i]
		if ref.Target.Kind != core.KindPull {
			continue
		}
		t := ref.Target
		if !t.HasRepo() {
			t.Repo = repo
		}
		if seen[t.String()] {
			continue
		}
		seen[t.String()] = true
		state := fmt.Sprint(ref.State)
		if ref.Draft {
			state += ", draft"
		}
		fmt.Fprintf(&b, "- %s %s (%s)\n", t, strings.ReplaceAll(ref.Title, "\n", " "), state)
	}
	if b.Len() == 0 {
		return "Pull requests referencing it: none\n"
	}
	return "Pull requests referencing it:\n\n" + b.String()
}
