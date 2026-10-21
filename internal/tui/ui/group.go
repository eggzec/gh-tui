package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// GroupModal is the choice of the levels a list is grouped by, as a modal:
// the fields it can be grouped by on the left, and the levels in force, in
// the order they nest, on the right. Space flips the order of a level.
// Enter applies the levels and closes the modal, and with none chosen the
// list is flat; esc closes it as it was.
type GroupModal struct {
	// verb starts the title: "Group", or "Sort".
	verb  string
	title string
	rows  int
	pick  orderpick.Model
	icons Icons
	// section is what the title names.
	section string
	apply   func([]orderpick.Choice) tea.Cmd
}

// NewGroupModal returns the modal that groups the list of section by fields,
// titled "Group · section". chosen are the levels in force, and apply takes
// those the user picks, which may be none.
func NewGroupModal(section string, fields []orderpick.Item, chosen []orderpick.Choice, keys orderpick.KeyMap, icons Icons, apply func([]orderpick.Choice) tea.Cmd) *GroupModal {
	return newLevelsModal("Group", "Grouped by", "Nothing: a flat list", section, fields, chosen, keys, icons, apply)
}

// NewSortLevelsModal returns the modal that sorts the list of section by
// several fields, one after the other, titled "Sort · section". chosen are
// the levels in force, and apply takes those the user picks, which may be
// none.
func NewSortLevelsModal(section string, fields []orderpick.Item, chosen []orderpick.Choice, keys orderpick.KeyMap, icons Icons, apply func([]orderpick.Choice) tea.Cmd) *GroupModal {
	return newLevelsModal("Sort", "Sorted by", "Nothing: the list's own order", section, fields, chosen, keys, icons, apply)
}

func newLevelsModal(verb, heading, empty, section string, fields []orderpick.Item, chosen []orderpick.Choice, keys orderpick.KeyMap, icons Icons, apply func([]orderpick.Choice) tea.Cmd) *GroupModal {
	m := &GroupModal{verb: verb, rows: len(fields), icons: icons, section: section, apply: apply}
	m.retitle()
	m.pick = orderpick.New(
		orderpick.WithItems(fields),
		orderpick.WithSingle(false),
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
func (m *GroupModal) retitle() {
	m.title = m.verb + m.icons.Separator + m.section
}

// Title implements Modal.
func (m *GroupModal) Title() string { return m.title }

// Update implements Modal. The picker's own messages carry its ID, so the
// modal ignores those of other pickers.
func (m *GroupModal) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case SettingsMsg:
		msg.ReloadIcons(&m.icons)
		m.retitle()
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
func (m *GroupModal) View() string { return m.pick.View() }

// SetSize implements Modal.
func (m *GroupModal) SetSize(width, height int) { m.pick.SetSize(width, height) }

// Fit implements Fitter.
func (m *GroupModal) Fit(maxWidth, maxHeight int) (width, height int) {
	return min(maxWidth, SortWidth), min(maxHeight, m.rows+sortAround)
}

// SetTheme implements Modal.
func (m *GroupModal) SetTheme(t Theme) { m.pick.SetStyles(t.OrderPick(m.icons)) }

// Commands implements Commanded: the modal takes only the commands that act on the app.
func (m *GroupModal) Commands() []string { return nil }

// KeyLayers implements Keyed: the keys of the picker, which types in the
// search of its left pane.
func (m *GroupModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{ContextHelp("order_pick", m.pick, m.pick.Capturing())}
}

var _ Actor = (*GroupModal)(nil)

// Act implements Actor. The quit key closes the modal without applying
// what the picker holds; every other intent is the app's to refuse while it
// is open.
func (m *GroupModal) Act(action string) (tea.Cmd, bool) {
	if action == ActQuit {
		return CloseModal(m), true
	}
	return nil, false
}
