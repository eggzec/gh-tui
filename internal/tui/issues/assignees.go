package issues

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/facets"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// assigneeRows is how many rows the picker counts besides the assignees: what
// a search of the people of the repository lists at most, which is the limit
// of facets.Service.People.
const assigneeRows = 20

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
	have := slices.Clone(m.issue.Assignees)
	// The search runs in a command, off the update loop, while the change reads
	// the people in it.
	var mu sync.Mutex
	people := map[string]core.User{}
	remember := func(users []core.User) {
		mu.Lock()
		defer mu.Unlock()
		for _, u := range users {
			people[u.Login] = u
		}
	}
	remember(have)

	o := &assigneesOverlay{
		Heading: "Assignees",
		Order:   strings.ToLower,
		On:      map[string]bool{},
		Was:     map[string]bool{},
		Rows:    len(have) + assigneeRows,
		Change: func(on, was map[string]bool) tea.Cmd {
			var add []core.User
			var remove []string
			mu.Lock()
			for login := range on {
				if !was[login] {
					add = append(add, people[login])
				}
			}
			mu.Unlock()
			for login := range was {
				if !on[login] {
					remove = append(remove, login)
				}
			}
			if len(add) == 0 && len(remove) == 0 {
				return nil
			}
			slices.SortFunc(add, func(a, b core.User) int { return strings.Compare(a.Login, b.Login) })
			slices.Sort(remove)
			return m.askAssignees(add, remove)
		},
	}
	for _, u := range have {
		o.On[u.Login], o.Was[u.Login] = true, true
	}

	// The assignees are in every answer, so that a search that does not find
	// them leaves them listed, and ticked.
	rows := func(found []core.User) []picker.Item {
		all := slices.Clone(have)
		for _, u := range found {
			if !slices.ContainsFunc(all, func(o core.User) bool { return strings.EqualFold(o.Login, u.Login) }) {
				all = append(all, u)
			}
		}
		remember(all)
		items := make([]picker.Item, 0, len(all))
		for _, it := range ui.PersonItems(all, "") {
			items = append(items, picker.Item{Title: it.Label, Detail: it.Detail, Value: it.Value})
		}
		return items
	}

	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	known := rows(nil)
	opts := []picker.Option{
		picker.WithContext(m.ctx),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(kindOn, kindOff),
		picker.WithTicks(),
		picker.WithMarkedFirst(true),
		picker.WithDetailMatch(true),
		picker.WithKeyMap(m.keys.pick),
		picker.WithKnown(known),
		picker.WithStyles(st),
	}
	var search picker.Search
	if f := m.facets; f != nil {
		repo := m.repo
		search = func(ctx context.Context, q picker.Query) ([]picker.Item, error) {
			found, err := f.People(ctx, facets.PeopleQuery{Repo: repo, Text: q.Text})
			if err != nil {
				return nil, err
			}
			return rows(found), nil
		}
	} else {
		opts = append(opts, picker.WithItems(known))
	}
	o.P = picker.New(search, opts...)
	o.P.SetMarked(o.Marks())
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
