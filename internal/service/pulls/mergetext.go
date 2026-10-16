package pulls

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/eggzec/gh-tui/internal/core"
)

// MergeTexts reads the commit messages GitHub would use to merge pull request
// number of repo. They depend on the viewer and on the head, and are read only
// when a merge is about to be made, so they are never cached.
func (s *Service) MergeTexts(ctx context.Context, repo core.RepoRef, number int) (core.MergeTexts, error) {
	t, err := s.api.PullMergeTexts(ctx, repo, number)
	if err != nil {
		// The dialog goes on with an empty message and says only the
		// cause, so the error is kept here.
		if !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "merge message not read", "span", "service.pulls", "repo", repo.String(), "number", number, "err", err.Error())
		}
		return core.MergeTexts{}, fmt.Errorf("read merge message of pull %s#%d: %w", repo, number, err)
	}
	return t, nil
}
