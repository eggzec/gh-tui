package filterform

import "slices"

// Tab is a tab of a form. A form whose spec has a sort shows the filters
// and the sort on two tabs; one without shows the filters alone, with no
// tabs.
type Tab int

const (
	// FiltersTab holds the fields.
	FiltersTab Tab = iota
	// SortTab holds what the list is sorted by, and the order.
	SortTab
	numTabs
)

// String returns the tab's name, as the form shows it.
func (t Tab) String() string {
	switch t {
	case FiltersTab:
		return "Filters"
	case SortTab:
		return "Sort"
	default:
		return "unknown"
	}
}

// tabNames are the names of the tabs, in order, tabHelpDescs how the help
// names them, and resetHelpDescs what reset does on each.
var (
	tabNames       = []string{FiltersTab.String(), SortTab.String()}
	tabHelpDescs   = [numTabs]string{"filters", "sort"}
	resetHelpDescs = [numTabs]string{"reset filters", "reset sort"}
)

// Rows of the Sort tab.
const (
	sortByRow = iota
	sortOrderRow
	sortRows
)

// Tab returns the tab on view.
func (m Model) Tab() Tab { return m.tab }

// Tabs returns the names of the tabs in order, or nil for a form without
// a sort, which has no tabs.
func (m Model) Tabs() []string {
	if !m.tabbed() {
		return nil
	}
	return tabNames
}

// SetTab shows tab t, with the focus on its first row. It closes an open
// editor and leaves the query line. A form without a sort has only the
// Filters tab, and ignores it.
func (m *Model) SetTab(t Tab) {
	if t < 0 || t >= numTabs || t != FiltersTab && !m.tabbed() {
		return
	}
	m.showTab(t)
	m.render()
}

// tabbed reports whether the form has tabs.
func (m *Model) tabbed() bool { return m.spec.Sort != nil }

// showTab shows tab t from its first row.
func (m *Model) showTab(t Tab) {
	m.closeEditor(true)
	if m.query.Focused() {
		m.query.Blur()
		m.syncQuery()
	}
	m.tab, m.row, m.top = t, 0, 0
}

// switchTab shows the tab delta after the one on view, wrapping around.
func (m *Model) switchTab(delta int) {
	if !m.tabbed() {
		return
	}
	n := int(numTabs)
	m.showTab(Tab(((int(m.tab)+delta)%n + n) % n))
}

// resetTab puts what the tab on view edits back to its defaults: the
// fields and the free text on the Filters tab, and the sort on the Sort
// tab.
func (m *Model) resetTab() {
	m.closeEditor(false)
	d := defaults(&m.spec)
	if m.tab == SortTab {
		m.state.sort = d.sort
	} else {
		m.state.values, m.state.free = d.values, nil
		m.fields = slices.Clone(m.fields)
		m.resetChips()
	}
	m.syncQuery()
}
