package pulls

import (
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"

	tea "charm.land/bubbletea/v2"
)

// openDetail opens the modal of pull request number of repo, on its Merge
// pane if onChecks is set. pr is the list item, shown until the detail
// arrives, or nil when there is none. The title names repo if showRepo is set
// or it isn't the one selected. back is the modal it replaces, which the back
// key returns to, or nil.
func (s *Section) openDetail(repo core.RepoRef, number int, pr *core.PullRequest, onChecks, showRepo bool, from ui.Pauser, back ui.Modal) tea.Cmd {
	return s.openPanes(repo, number, pr, onChecks, showRepo, from, back)
}

// detailKey names pull request number of repo to the reads ahead.
func detailKey(repo core.RepoRef, number int) details.Key {
	return details.Key{Pull: true, Repo: repo, Number: number}
}
