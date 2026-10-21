package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// Size of the sort modal: wide enough for both panes side by side, and tall
// enough for the options of a list and the frames around them.
const (
	// SortWidth is the width of the modal of a sort.
	SortWidth = 80
	// sortAround is what the panes show besides the options: their frames,
	// the heading, the count and the filter line.
	sortAround = 7
)

// SortModal is the sort of a Filterable as a modal: the ways the list can be
// sorted on the left, and the one in force on the right. GitHub takes one
// sort, so choosing another replaces it. Enter applies it and closes the
// modal, and with nothing chosen the list shows in its own order; esc closes
// it as it was.
type SortModal struct {
	title  string
	target Filterable
	spec   filterform.Spec
	query  string
	rows   int
	pick   orderpick.Model
	icons  Icons
	// theme is the last one set, if themed, so that new icons restyle the picker.
	theme  Theme
	themed bool
	// section and subject are what the title names.
	section, subject string
}

// NewSortModal returns the modal that sorts target, which edits f, titled
// "Sort · section · subject". The options are those of f's sort, but best
// match, which is nothing chosen, and the one the query holds is chosen.
func NewSortModal(section string, target Filterable, f Filter, keys orderpick.KeyMap, icons Icons) *SortModal {
	m := &SortModal{target: target, spec: f.Spec, query: f.Query, icons: icons, section: section, subject: f.Subject}
	m.retitle()
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
	m.rows = len(items)
	opts := []orderpick.Option{
		orderpick.WithItems(items),
		orderpick.WithSingle(true),
		orderpick.WithKeyMap(keys),
		orderpick.WithHeading("Sorted by"),
		orderpick.WithEmptyText("Nothing: the list's own order"),
	}
	if cur := f.Spec.SortOf(f.Query); cur.By != "" {
		opts = append(opts, orderpick.WithChosen(orderpick.Choice{Value: cur.By, Desc: cur.Desc}))
	}
	m.pick = orderpick.New(opts...)
	// Focusing the left pane, where the picker starts, needs no command.
	_ = m.pick.Focus()
	return m
}

// retitle words the title with the separator of the icons.
func (m *SortModal) retitle() {
	m.title = "Sort" + m.icons.Separator + m.section
	if m.subject != "" {
		m.title += m.icons.Separator + m.subject
	}
}

// Title implements Modal.
func (m *SortModal) Title() string { return m.title }

// Update implements Modal. The picker's own messages carry its ID, so the
// modal ignores those of other pickers.
func (m *SortModal) Update(msg tea.Msg) tea.Cmd {
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
		var so filterform.Sort
		if len(msg.Choices) > 0 {
			by, _ := msg.Choices[0].Value.(string)
			so = filterform.Sort{By: by, Desc: msg.Choices[0].Desc}
		}
		applied := filterform.AppliedMsg{Query: m.spec.WithSort(m.query, so), Sort: so}
		return tea.Batch(LeaveModal(m), m.target.ApplyFilter(applied))
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
func (m *SortModal) View() string { return m.pick.View() }

// SetSize implements Modal.
func (m *SortModal) SetSize(width, height int) { m.pick.SetSize(width, height) }

// Fit implements Fitter.
func (m *SortModal) Fit(maxWidth, maxHeight int) (width, height int) {
	return min(maxWidth, SortWidth), min(maxHeight, m.rows+sortAround)
}

// SetTheme implements Modal.
func (m *SortModal) SetTheme(t Theme) {
	m.theme, m.themed = t, true
	m.pick.SetStyles(t.OrderPick(m.icons))
}

// Commands implements Commanded: the sort takes only the commands that act on the app.
func (m *SortModal) Commands() []string { return nil }

// KeyLayers implements Keyed: the keys of the picker, which types in the
// search of its left pane.
func (m *SortModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{ContextHelp("order_pick", m.pick, m.pick.Capturing())}
}

var _ Actor = (*SortModal)(nil)

// Act implements Actor. The quit key closes the modal without applying
// what the picker holds; every other intent is the app's to refuse while it
// is open.
func (m *SortModal) Act(action string) (tea.Cmd, bool) {
	if action == ActQuit {
		return CloseModal(m), true
	}
	return nil, false
}
