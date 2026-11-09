package pulls

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/facets"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// assigneeEditor is what the assignees picker needs to change the assignees.
type assigneeEditor interface {
	// EditAssignees shows the people in add as assigned to the pull request,
	// and those whose logins are in remove as taken off, in the cache at once.
	// The returned Op sends the change.
	EditAssignees(repo core.RepoRef, number int, add []core.User, remove []string) *optimistic.Op
}

// openAssignees opens the picker of the assignees of the pull request. It
// searches the people the repository offers as the user types, and keeps the
// current assignees listed, ticked, whatever the search returns.
func (m *panesModal) openAssignees() tea.Cmd {
	if !m.loaded || m.top() != nil {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActAssign, &m.detail.Issue); refused {
		return cmd
	}
	m.showAssignees()
	o, ok := m.top().(*assigneesOverlay)
	if !ok {
		return nil
	}
	return tea.Batch(o.P.Focus(), o.P.Init())
}

// assigneesOverlay is the checklist of the assignees of the pull request,
// keyed by login.
type assigneesOverlay = prpanes.TickOverlay[string]

// showAssignees opens the picker over the people of the repository.
func (m *panesModal) showAssignees() {
	spec := prpanes.AssigneesSpec{
		Ctx:    m.ctx,
		Have:   m.detail.Assignees,
		Styles: m.theme.Picker(m.icons),
		Keys:   m.keys.pick,
		On:     kindOn,
		Off:    kindOff,
		Apply:  m.applyAssignees,
	}
	if f := m.list.facets; f != nil {
		repo := m.repo
		spec.Search = func(ctx context.Context, text string) ([]core.User, error) {
			return f.People(ctx, facets.PeopleQuery{Repo: repo, Text: text})
		}
	}
	m.open(prpanes.NewAssignees(spec))
}

// applyAssignees changes the assignees of the pull request, which the pane of
// the people shows at once, and reads it again once the change went through or
// was rolled back.
func (m *panesModal) applyAssignees(add []core.User, remove []string) tea.Cmd {
	op := m.svc.EditAssignees(m.repo, m.number, add, remove)
	if d, ok := m.svc.CachedGet(m.repo, m.number); ok {
		m.detail = d
		m.refreshInvolved()
	}
	return m.sendOp(op, "assignees on #"+strconv.Itoa(m.number))
}
