package search

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
)

// PreviewServices holds what the lazy preview of each kind of hit reads
// from. A kind whose service is nil has no lazy preview.
type PreviewServices struct {
	Repos   RepoPreviews
	Users   UserPreviews
	Commits CommitPreviews
	Issues  IssuePreviews
	Pulls   PullPreviews
	// Ellipsis marks what a preview leaves out, such as the files of a
	// commit beyond the first page.
	Ellipsis string
}

// NewPreviewReader returns the PreviewReader that hands each kind of hit to
// its own reader, one file per kind. A kind without a reader gets nil, so
// its preview is the hit's own data.
func NewPreviewReader(s PreviewServices) PreviewReader {
	return func(h core.SearchHit) func(ctx context.Context) (string, error) {
		// One line per kind that reads more of its preview.
		switch h.Kind {
		case core.SearchRepos:
			return repoReader(s.Repos, h)
		case core.SearchUsers:
			return userReader(s.Users, h)
		case core.SearchCommits:
			return commitReader(s.Commits, s.Ellipsis, h)
		case core.SearchIssues:
			return issueReader(s.Issues, h)
		case core.SearchPulls:
			return pullReader(s.Pulls, s.Ellipsis, h)
		default:
			return nil
		}
	}
}
