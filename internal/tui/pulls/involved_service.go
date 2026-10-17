package pulls

import (
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// reviewRequester is what the involved pane needs to ask for a review again.
type reviewRequester interface {
	// RequestReviews shows the people as asked in the cache at once. The
	// returned Op sends the request.
	RequestReviews(repo core.RepoRef, number int, who []pulls.Reviewer) *optimistic.Op
}
