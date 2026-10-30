package actions

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// viewWorkflow previews the workflow file of the run, as it was at the
// commit the run ran on. The workflows are read first if they aren't yet,
// since they know the file's path.
func (m *Modal) viewWorkflow() tea.Cmd {
	if !m.hasRun {
		return nil
	}
	if m.workflows.loaded {
		return m.openWorkflowFile()
	}
	m.workflows.viewing = true
	if m.workflows.loading {
		return nil
	}
	m.workflows.loading = true
	return tea.Batch(m.readWorkflows(false), m.spin.Start())
}

// openWorkflowFile sends the file of the run's workflow to the preview.
func (m *Modal) openWorkflowFile() tea.Cmd {
	path, ok := m.workflowPath(m.run)
	if !ok || m.run.HeadSHA == "" {
		return ui.Notify(toast.Warning, "There is no workflow file for this run.")
	}
	msg := ui.OpenFileMsg{Repo: m.repo, Path: path, Ref: m.run.HeadSHA}
	return func() tea.Msg { return msg }
}

// workflowPath returns the path of the file of the workflow of r, if the
// repository has one: a dynamic workflow has no file.
func (m *Modal) workflowPath(r core.Run) (string, bool) {
	for _, w := range m.workflows.items {
		if w.ID == r.WorkflowID {
			return w.Path, w.Path != "" && !strings.HasPrefix(w.Path, "dynamic/")
		}
	}
	return "", false
}
