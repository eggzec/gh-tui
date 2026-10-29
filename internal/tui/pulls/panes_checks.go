package pulls

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// actionsOn returns the message that opens the Actions modal of repo on the
// job of a check row: among the runs of the commit the row belongs to, on its
// run, with the jobs focused on the job. Leaving the Actions modal returns to
// back, the modal that holds the row.
func actionsOn(repo core.RepoRef, msg checktree.OpenJobMsg, back ui.Modal) tea.Msg {
	return ui.OpenActionsMsg{
		Repo:   repo,
		Filter: core.RunFilter{HeadSHA: msg.SHA},
		At:     ui.ActionsAt{Run: msg.RunID, Job: msg.JobID},
		Over:   back,
	}
}
