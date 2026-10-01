package ui

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// OnlineMsg tells the sections that GitHub answers again after the app
// couldn't reach it, or that the last rate limit lifted. Each reads
// again, once, what it shows that failed meanwhile (Unreached), such as a
// list whose page didn't load; the files section also reads again a
// listing it was served kept. What was served fresh costs no request.
type OnlineMsg struct{}

// Unreached reports whether err is of a read that got no answer from
// GitHub, or only a server error, which reading again once GitHub
// answers may mend. A refusal, such as a missing repository, would only
// be refused again.
func Unreached(err error) bool {
	return errors.Is(err, core.ErrOffline) || errors.Is(err, core.ErrUnavailable)
}

// Retrier is a list that reads again what failed of it, such as a feed.
type Retrier interface {
	Err() error
	Retry() tea.Cmd
}

// RetryUnreached reads again what failed of r, as a section does on an
// OnlineMsg, if it failed for want of an answer from GitHub, and returns
// nil otherwise.
func RetryUnreached(r Retrier) tea.Cmd {
	if !Unreached(r.Err()) {
		return nil
	}
	return r.Retry()
}
