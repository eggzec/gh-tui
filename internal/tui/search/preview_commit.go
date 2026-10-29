package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// CommitPreviews reads what the preview of a commit shows beyond its hit.
type CommitPreviews interface {
	Commit(ctx context.Context, repo core.RepoRef, sha string) (core.CommitPreview, error)
}

// commitReader returns the read of a commit hit's preview.
func commitReader(cp CommitPreviews, ellipsis string, h core.SearchHit) func(ctx context.Context) (string, error) {
	if cp == nil {
		return nil
	}
	repo, sha := h.Commit.Repo, h.Commit.SHA
	return func(ctx context.Context) (string, error) {
		p, err := cp.Commit(ctx, repo, sha)
		if err != nil {
			return "", err
		}
		return commitExtra(p, ellipsis), nil
	}
}

// commitExtra is what a read of a commit adds under the hit's own text, as
// Markdown: the committer, the message's body, the lines each file changed
// and the patch, which the pager shows as a highlighted diff.
func commitExtra(p core.CommitPreview, ellipsis string) string {
	d := p.Detail
	var b strings.Builder
	if c := d.Committer; c.Name != "" {
		fmt.Fprintf(&b, "- Committer: %s", c.Name)
		if !c.Date.IsZero() {
			fmt.Fprintf(&b, " (%s)", c.Date.Format("2006-01-02 15:04"))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "- Changes: +%d -%d in %d files\n", d.Stats.Additions, d.Stats.Deletions, len(d.Files))
	if d.Body != "" {
		b.WriteString("\n")
		b.WriteString(d.Body)
		b.WriteString("\n")
	}
	if len(d.Files) > 0 {
		b.WriteString("\n")
		for _, f := range d.Files {
			fmt.Fprintf(&b, "- `%s` +%d -%d\n", f.Path, f.Additions, f.Deletions)
		}
		if d.FilesNext != "" || d.FilesTruncated {
			b.WriteString("- " + ellipsis + "\n")
		}
	}
	switch {
	case p.PatchTooLarge:
		b.WriteString("\n---\n\nPatch too large to show.\n")
	case p.Patch != "":
		fence := strings.Repeat("`", max(3, longestRun(p.Patch, '`')+1))
		fmt.Fprintf(&b, "\n---\n\n%sdiff\n%s\n%s\n", fence, strings.TrimRight(p.Patch, "\n"), fence)
	}
	return b.String()
}

// longestRun returns the length of the longest run of c in s.
func longestRun(s string, c byte) int {
	best, run := 0, 0
	for i := range len(s) {
		if s[i] == c {
			run++
			best = max(best, run)
		} else {
			run = 0
		}
	}
	return best
}
