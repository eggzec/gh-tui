package pulls

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
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// assigneeEditor is what the assignees picker needs to change the assignees.
type assigneeEditor interface {
	// EditAssignees shows the people in add as assigned to the pull request,
	// and those whose logins are in remove as taken off, in the cache at once.
	// The returned Op sends the change.
	EditAssignees(repo core.RepoRef, number int, add []core.User, remove []string) *optimistic.Op
}

// assigneeRows is how many rows the picker counts besides the assignees: what
// a search of the people of the repository lists at most, which is the limit
// of facets.Service.People (peopleLimit there, unexported).
const assigneeRows = 20

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
	return tea.Batch(o.p.Focus(), o.p.Init())
}

// assigneesOverlay is the checklist of the assignees of the pull request,
// keyed by login.
type assigneesOverlay = tickOverlay[string]

// showAssignees opens the picker over the people of the repository.
func (m *panesModal) showAssignees() {
	have := slices.Clone(m.detail.Assignees)
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
		heading: "Assignees",
		order:   strings.ToLower,
		on:      map[string]bool{},
		was:     map[string]bool{},
		rows:    len(have) + assigneeRows,
		change: func(on, was map[string]bool) tea.Cmd {
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
			return m.applyAssignees(add, remove)
		},
	}
	for _, u := range have {
		o.on[u.Login], o.was[u.Login] = true, true
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

	var search picker.Search
	opts := m.pickerOptions(rows(nil))
	if f := m.list.facets; f != nil {
		repo := m.repo
		search = func(ctx context.Context, q picker.Query) ([]picker.Item, error) {
			found, err := f.People(ctx, facets.PeopleQuery{Repo: repo, Text: q.Text})
			if err != nil {
				return nil, err
			}
			return rows(found), nil
		}
	} else {
		opts = append(opts, picker.WithItems(rows(nil)))
	}
	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	opts = append(opts, picker.WithStyles(st))
	o.p = picker.New(search, opts...)
	o.p.SetMarked(o.marks())
	m.open(o)
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
