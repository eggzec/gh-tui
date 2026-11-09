package issues

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/facets"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// assigneesOverlay is the checklist of the assignees of the issue, keyed by
// login.
type assigneesOverlay = prpanes.TickOverlay[string]

// openAssignees opens the picker of the assignees of the issue. It searches
// the people the repository offers as the user types, and keeps the current
// assignees listed, ticked, whatever the search returns.
func (m *detailModal) openAssignees() tea.Cmd {
	if !m.loaded {
		return ui.Notify(toast.Info, "Still reading #"+strconv.Itoa(m.number)+".")
	}
	if m.busy() {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActAssign, &m.issue); refused {
		return cmd
	}
	o := m.showAssignees()
	return tea.Batch(o.P.Focus(), o.P.Init())
}

// showAssignees opens the picker over the people of the repository. Applying
// asks to confirm the change.
func (m *detailModal) showAssignees() *assigneesOverlay {
	spec := prpanes.AssigneesSpec{
		Ctx:    m.ctx,
		Have:   m.issue.Assignees,
		Styles: m.theme.Picker(m.icons),
		Keys:   m.keys.pick,
		On:     kindOn,
		Off:    kindOff,
		Apply:  m.askAssignees,
	}
	if f := m.facets; f != nil {
		repo := m.repo
		spec.Search = func(ctx context.Context, text string) ([]core.User, error) {
			return f.People(ctx, facets.PeopleQuery{Repo: repo, Text: text})
		}
	}
	o := prpanes.NewAssignees(spec)
	m.over = o
	return o
}

// askAssignees asks to assign and unassign people on the issue, naming each.
func (m *detailModal) askAssignees(add []core.User, remove []string) tea.Cmd {
	c, ok, refusal := m.assignees(add, remove)
	if !ok {
		return refusal
	}
	return m.asks(ui.Recheck(c.Question, "#"+strconv.Itoa(m.number), func() (confirm.Ask, bool, tea.Cmd) {
		return m.assignees(add, remove)
	}))
}

// assignees returns assigning and unassigning the people as a question, or ok
// unset when there is nothing to change or the viewer may not, which refusal
// then says. Its run shows the change in the cache at once and sends it.
func (m *detailModal) assignees(add []core.User, remove []string) (c confirm.Ask, ok bool, refusal tea.Cmd) {
	if !m.loaded || (len(add) == 0 && len(remove) == 0) {
		return confirm.Ask{}, false, nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActAssign, &m.issue); refused {
		return confirm.Ask{}, false, cmd
	}
	logins := make([]string, len(add))
	for i, u := range add {
		logins[i] = u.Login
	}
	n := m.number
	num := "#" + strconv.Itoa(n)
	return confirm.Ask{
		Question: assigneeQuestion(num, logins, remove),
		Run: func() tea.Cmd {
			op := m.svc.EditAssignees(m.repo, n, add, remove)
			return tea.Batch(m.reload(), m.changed(), m.send(op, "assignees on "+num))
		},
	}, true, nil
}

// assigneeQuestion asks to assign and unassign people on issue num, such as
// "Assign @a and unassign @b on #12?".
func assigneeQuestion(num string, added, removed []string) string {
	switch {
	case len(removed) == 0:
		return "Assign " + mentions(added) + " on " + num + "?"
	case len(added) == 0:
		return "Unassign " + mentions(removed) + " on " + num + "?"
	}
	return "Assign " + mentions(added) + " and unassign " + mentions(removed) + " on " + num + "?"
}

// mentions writes logins as @login, joined on one line.
func mentions(logins []string) string {
	out := make([]string, len(logins))
	for i, l := range logins {
		out[i] = "@" + l
	}
	return oneLine(out)
}
