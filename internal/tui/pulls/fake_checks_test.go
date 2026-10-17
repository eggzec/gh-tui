package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

const checkSHA = "0123456789abcdef0123456789abcdef01234567"

func TestOpenJobMsgOpensTheActionsOnTheJob(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	for _, focus := range []bool{false, true} {
		m.zoom = focus // the pane that sent it doesn't matter
		cmd := m.Update(checktree.OpenJobMsg{SHA: checkSHA, RunID: 7, JobID: 9})
		if cmd == nil {
			t.Fatal("OpenJobMsg returned no command")
		}
		got, ok := cmd().(ui.OpenActionsMsg)
		if !ok {
			t.Fatalf("OpenJobMsg asked for %T, want OpenActionsMsg", cmd())
		}
		if got.Repo != m.repo || got.Filter != (core.RunFilter{HeadSHA: checkSHA}) ||
			got.At != (ui.ActionsAt{Run: 7, Job: 9, Jobs: true}) || got.Back != ui.Modal(m) {
			t.Errorf("OpenActionsMsg = %+v, want the job of the commit, back to the modal", got)
		}
	}
}

func TestOpenCheckMsgDoesNothing(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	if cmd := m.Update(checktree.OpenCheckMsg{SHA: checkSHA, CheckID: 1, SuiteID: 2}); cmd != nil {
		t.Error("OpenCheckMsg returned a command")
	}
}
