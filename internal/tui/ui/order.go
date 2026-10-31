package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// Size of the order modal: wide enough for both panes side by side, and tall
// enough for the options of a list and the frames around them.
const (
	// SortWidth is the width of the modal of a sort or a grouping.
	SortWidth = 80
	// sortAround is what the panes show besides the options: their frames,
	// the heading, the count and the filter line.
	sortAround = 7
)

// DefaultSort is the sort GitHub applies to a list when its query holds none.
const DefaultSort = "sort:updated-desc"

// OrderModal is the order of a list as a modal: the fields it can be sorted
// or grouped by on the left, and those in force on the right. A sort of a
// Filterable takes one field, as GitHub does, so choosing another replaces
// it; the levels of a grouping, or of a sort of loaded rows, nest in the
// order chosen, and space flips the order of a level. Enter applies what is
// chosen and closes the modal, and with nothing chosen the list shows in its
// own order; esc closes it as it was.
type OrderModal struct {
	// verb starts the title: "Group", or "Sort".
	verb  string
	title string
	rows  int
	pick  orderpick.Model
	icons Icons
	// theme is the last one set, if themed, so that new icons restyle the picker.
	theme  Theme
	themed bool
	// section and subject are what the title names.
	section, subject string
	apply            func([]orderpick.Choice) tea.Cmd
}

// NewSortModal returns the modal that sorts target, which edits f, titled
// "Sort · section · subject". The options are those of f's sort, but best
// match, which is nothing chosen, and the one the query holds is chosen.
func NewSortModal(section string, target Filterable, f Filter, keys orderpick.KeyMap, icons Icons) *OrderModal {
	var items []orderpick.Item
	if f.Spec.Sort != nil {
		for _, o := range f.Spec.Sort.Options {
			if o.Value == "" {
				continue
			}
			items = append(items, orderpick.Item{
				Value: o.Value, Title: o.Label, AscLabel: o.Asc, DescLabel: o.Desc, Desc: !o.Ascending,
			})
		}
	}
	var chosen []orderpick.Choice
	if cur := f.Spec.SortOf(f.Query); cur.By != "" {
		chosen = append(chosen, orderpick.Choice{Value: cur.By, Desc: cur.Desc})
	}
	apply := func(choices []orderpick.Choice) tea.Cmd {
		var so filterform.Sort
		if len(choices) > 0 {
			by, _ := choices[0].Value.(string)
			so = filterform.Sort{By: by, Desc: choices[0].Desc}
		}
		return target.ApplyFilter(filterform.AppliedMsg{Query: f.Spec.WithSort(f.Query, so), Sort: so})
	}
	m := newOrderModal("Sort", "Sorted by", "Nothing: the list's own order", section, items, chosen, true, keys, icons, apply)
	m.subject = f.Subject
	m.retitle()
	return m
}

// NewGroupModal returns the modal that groups the list of section by fields,
// titled "Group · section". chosen are the levels in force, and apply takes
// those the user picks, which may be none.
func NewGroupModal(section string, fields []orderpick.Item, chosen []orderpick.Choice, keys orderpick.KeyMap, icons Icons, apply func([]orderpick.Choice) tea.Cmd) *OrderModal {
	return newOrderModal("Group", "Grouped by", "Nothing: a flat list", section, fields, chosen, false, keys, icons, apply)
}

// NewSortLevelsModal returns the modal that sorts the list of section by
// several fields, one after the other, titled "Sort · section". chosen are
// the levels in force, and apply takes those the user picks, which may be
// none.
func NewSortLevelsModal(section string, fields []orderpick.Item, chosen []orderpick.Choice, keys orderpick.KeyMap, icons Icons, apply func([]orderpick.Choice) tea.Cmd) *OrderModal {
	return newOrderModal("Sort", "Sorted by", "Nothing: the list's own order", section, fields, chosen, false, keys, icons, apply)
}

func newOrderModal(verb, heading, empty, section string, fields []orderpick.Item, chosen []orderpick.Choice, single bool, keys orderpick.KeyMap, icons Icons, apply func([]orderpick.Choice) tea.Cmd) *OrderModal {
	m := &OrderModal{verb: verb, rows: len(fields), icons: icons, section: section, apply: apply}
	m.retitle()
	m.pick = orderpick.New(
		orderpick.WithItems(fields),
		orderpick.WithSingle(single),
		orderpick.WithKeyMap(keys),
		orderpick.WithHeading(heading),
		orderpick.WithEmptyText(empty),
		orderpick.WithChosen(chosen...),
	)
	// Focusing the left pane, where the picker starts, needs no command.
	_ = m.pick.Focus()
	return m
}

// retitle words the title with the separator of the icons.
func (m *OrderModal) retitle() {
	m.title = m.verb + m.icons.Separator + m.section
	if m.subject != "" {
		m.title += m.icons.Separator + m.subject
	}
}

// Title implements Modal.
func (m *OrderModal) Title() string { return m.title }

// Update implements Modal. The picker's own messages carry its ID, so the
// modal ignores those of other pickers.
func (m *OrderModal) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case SettingsMsg:
		msg.ReloadIcons(&m.icons)
		m.retitle()
		// The picker's styles hold the icons too; the theme that follows
		// would draw them, but a modal open meanwhile keeps its own.
		if m.themed {
			m.pick.SetStyles(m.theme.OrderPick(m.icons))
		}
		return nil
	case orderpick.ChosenMsg:
		if msg.ID != m.pick.ID() {
			return nil
		}
		return tea.Batch(LeaveModal(m), m.apply(msg.Choices))
	case orderpick.CancelMsg:
		if msg.ID != m.pick.ID() {
			return nil
		}
		return LeaveModal(m)
	}
	var cmd tea.Cmd
	m.pick, cmd = m.pick.Update(msg)
	return cmd
}

// View implements Modal.
func (m *OrderModal) View() string { return m.pick.View() }

// SetSize implements Modal.
func (m *OrderModal) SetSize(width, height int) { m.pick.SetSize(width, height) }

// Fit implements Fitter.
func (m *OrderModal) Fit(maxWidth, maxHeight int) (width, height int) {
	return min(maxWidth, SortWidth), min(maxHeight, m.rows+sortAround)
}

// SetTheme implements Modal.
func (m *OrderModal) SetTheme(t Theme) {
	m.theme, m.themed = t, true
	m.pick.SetStyles(t.OrderPick(m.icons))
}

// Commands implements Commanded: the modal takes only the commands that act on the app.
func (m *OrderModal) Commands() []string { return nil }

// KeyLayers implements Keyed: the keys of the picker, which types in the
// search of its left pane.
func (m *OrderModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{ContextHelp("order_pick", m.pick, m.pick.Capturing())}
}

var _ Actor = (*OrderModal)(nil)

// Act implements Actor. The quit key closes the modal without applying
// what the picker holds; every other intent is the app's to refuse while it
// is open.
func (m *OrderModal) Act(action string) (tea.Cmd, bool) {
	if action == ActQuit {
		return CloseModal(m), true
	}
	return nil, false
}
