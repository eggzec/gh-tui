package search

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// RepoPreviews reads what the preview of a repository shows beyond its hit.
type RepoPreviews interface {
	Preview(ctx context.Context, ref core.RepoRef) (core.RepoPreview, error)
}

// repoReader returns the read of a repository hit's preview.
func repoReader(rp RepoPreviews, h core.SearchHit) func(ctx context.Context) (string, error) {
	if rp == nil {
		return nil
	}
	ref := h.Repo.Ref
	return func(ctx context.Context) (string, error) {
		p, err := rp.Preview(ctx, ref)
		if err != nil {
			return "", err
		}
		return repoExtra(p), nil
	}
}

// repoExtra is what a read of a repository adds under the hit's own text, as
// Markdown: the open counts and the latest release, then the README.
func repoExtra(p core.RepoPreview) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Open issues: %d\n- Open pull requests: %d\n", p.OpenIssues, p.OpenPulls)
	if p.Release.Tag != "" {
		fmt.Fprintf(&b, "- Latest release: %s", p.Release.Tag)
		if at := cmpTime(p.Release.PublishedAt, p.Release.CreatedAt); !at.IsZero() {
			fmt.Fprintf(&b, " (%s)", at.Format("2006-01-02"))
		}
		b.WriteString("\n")
	}
	if p.Readme != "" {
		b.WriteString("\n---\n\n" + p.Readme)
	}
	return b.String()
}

// cmpTime returns a, or b if a is not set.
func cmpTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	return a
}
