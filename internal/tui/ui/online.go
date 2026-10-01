package ui

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// OnlineMsg tells the sections that GitHub answers again after the app
// couldn't reach it, or that the last rate limit lifted. Each reads
// again, once, what it shows that failed meanwhile (Unreached), such as a
// list whose page didn't load, and what it was served kept meanwhile,
// such as a page kept while GitHub rate limited its read. What was served
// fresh costs no request.
type OnlineMsg struct{}

// Unreached reports whether err is of a read that got no answer from
// GitHub, only a server error, or a rate limit, which reading again once
// GitHub answers, or the limit lifts, may mend. A read that a limit still
// holds fails again at once, without a request. A refusal, such as a
// missing repository, would only be refused again.
func Unreached(err error) bool {
	return errors.Is(err, core.ErrOffline) || errors.Is(err, core.ErrUnavailable) || errors.Is(err, core.ErrRateLimited)
}

// Retrier is a list that reads again what failed of it, such as a feed.
type Retrier interface {
	Err() error
	Retry() tea.Cmd
}

// RetryUnreached reads again, as a section does on an OnlineMsg, what
// failed of r for want of an answer from GitHub (Unreached), and what r
// was served kept, if it tells that, and returns nil if neither.
func RetryUnreached(r Retrier) tea.Cmd {
	var cmd tea.Cmd
	if Unreached(r.Err()) {
		cmd = r.Retry()
	}
	if k, ok := r.(keptRetrier); ok {
		cmd = tea.Batch(cmd, k.RetryKept())
	}
	return cmd
}

// keptRetrier is a list that reads again what it was served kept, such
// as a feed's pages kept while GitHub couldn't be reached or rate limited
// the read.
type keptRetrier interface {
	RetryKept() tea.Cmd
}
