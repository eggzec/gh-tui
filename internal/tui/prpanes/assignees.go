package prpanes

import (
	"context"
	"slices"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// assigneeRows is how many rows the picker counts besides the assignees: what
// a search of the people of the repository lists at most, which is the limit
// of facets.Service.People (peopleLimit there, unexported).
const assigneeRows = 20

// AssigneesSpec is what the checklist of assignees needs from a modal.
type AssigneesSpec struct {
	Ctx  context.Context
	Have []core.User
	// Search lists the people of the repository whose logins match text. Nil
	// leaves the picker to the Have rows.
	Search func(ctx context.Context, text string) ([]core.User, error)
	Styles picker.Styles
	Keys   picker.KeyMap
	// On and Off are the marks of a ticked and an unticked row.
	On, Off string
	// Apply gets the people to add and the logins to remove, sorted, once
	// enter finds a change.
	Apply func(add []core.User, remove []string) tea.Cmd
}

// NewAssignees builds the checklist of the assignees, keyed by login, with the
// current assignees ticked. They are in every answer, so that a search that
// does not find them leaves them listed, and ticked.
func NewAssignees(spec AssigneesSpec) *TickOverlay[string] {
	have := slices.Clone(spec.Have)
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

	o := &TickOverlay[string]{
		Heading: "Assignees",
		Order:   strings.ToLower,
		On:      map[string]bool{},
		Was:     map[string]bool{},
		Rows:    len(have) + assigneeRows,
		Change: func(on, was map[string]bool) tea.Cmd {
			mu.Lock()
			add, remove := AssigneeDiff(on, was, people)
			mu.Unlock()
			if len(add) == 0 && len(remove) == 0 {
				return nil
			}
			return spec.Apply(add, remove)
		},
	}
	for _, u := range have {
		o.On[u.Login], o.Was[u.Login] = true, true
	}

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

	st := spec.Styles
	st.Frame = lipgloss.NewStyle()
	known := rows(nil)
	opts := []picker.Option{
		picker.WithContext(spec.Ctx),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(spec.On, spec.Off),
		picker.WithTicks(),
		picker.WithMarkedFirst(true),
		picker.WithDetailMatch(true),
		picker.WithKeyMap(spec.Keys),
		picker.WithKnown(known),
		picker.WithStyles(st),
	}
	var search picker.Search
	if spec.Search != nil {
		search = func(ctx context.Context, q picker.Query) ([]picker.Item, error) {
			found, err := spec.Search(ctx, q.Text)
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
	return o
}

// AssigneeDiff is the people to add, found in people by login, and the logins
// to remove, to go from the ticks in was to those in on, each sorted by login.
func AssigneeDiff(on, was map[string]bool, people map[string]core.User) (add []core.User, remove []string) {
	for login := range on {
		if !was[login] {
			add = append(add, people[login])
		}
	}
	for login := range was {
		if !on[login] {
			remove = append(remove, login)
		}
	}
	slices.SortFunc(add, func(a, b core.User) int { return strings.Compare(a.Login, b.Login) })
	slices.Sort(remove)
	return add, remove
}
