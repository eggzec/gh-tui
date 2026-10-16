package pulls

import (
	"context"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
)

// reviewCall is one request or withdrawal that reached the API.
type reviewCall struct {
	number        int
	logins, teams []string
}

// reviewersFake backs the review requests and the team candidates of fakeAPI.
type reviewersFake struct {
	reviewMu sync.Mutex
	// requested and removed are the calls made, and reviewErr fails them.
	requested, removed []reviewCall
	reviewErr          error
	// teamReads counts the reads of the candidates; teams answers them, and
	// asked holds the texts they were sent.
	teamReads int
	teams     func(ctx context.Context, org, text string, first int) ([]core.Team, error)
	asked     []string
}

func (f *reviewersFake) RequestReviewers(_ context.Context, _ core.RepoRef, number int, logins, teams []string) error {
	f.reviewMu.Lock()
	defer f.reviewMu.Unlock()
	f.requested = append(f.requested, reviewCall{number, logins, teams})
	return f.reviewErr
}

func (f *reviewersFake) RemoveReviewers(_ context.Context, _ core.RepoRef, number int, logins, teams []string) error {
	f.reviewMu.Lock()
	defer f.reviewMu.Unlock()
	f.removed = append(f.removed, reviewCall{number, logins, teams})
	return f.reviewErr
}

func (f *reviewersFake) ReviewerTeams(ctx context.Context, org, text string, first int) ([]core.Team, error) {
	f.reviewMu.Lock()
	f.teamReads++
	f.asked = append(f.asked, org+"?"+text)
	f.reviewMu.Unlock()
	return f.teams(ctx, org, text, first)
}

// reviewCalls returns how many requests and withdrawals reached the API.
func (f *reviewersFake) reviewCalls() int {
	f.reviewMu.Lock()
	defer f.reviewMu.Unlock()
	return len(f.requested) + len(f.removed)
}
