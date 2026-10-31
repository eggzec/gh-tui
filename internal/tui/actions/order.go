package actions

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// The runs are grouped under headers and sorted over the pages loaded so
// far, by the same levels the notifications offer.

// groupField is a way to group the runs: what a run is filed under.
type groupField struct {
	name  string
	label string
	// key is the group a run is in, "" for none.
	key func(r core.Run) string
}

// groupFields are the ways to group the runs, in the order the modal lists
// them.
var groupFields = []groupField{
	{"workflow", "Workflow", func(r core.Run) string { return r.Name }},
	{"event", "Event", func(r core.Run) string { return r.Event }},
	{"branch", "Branch", func(r core.Run) string { return r.Branch }},
	{"status", "Status", statusOf},
	{"actor", "Actor", func(r core.Run) string { return r.Actor }},
}

// defaultGroups are the levels the runs start grouped by.
func defaultGroups() []orderpick.Choice {
	return []orderpick.Choice{{Value: "workflow"}}
}

// statusOf is how the run stands: how it ended, or where it is.
func statusOf(r core.Run) string {
	if r.Conclusion != core.ConclusionNone {
		return string(r.Conclusion)
	}
	return string(r.Status)
}

// createdField is the sort by the time a run was created. GitHub lists the
// runs newest first, so that is its order unless it is flipped.
const createdField = "created"

// groupItems are the fields the group modal offers.
func groupItems() []orderpick.Item {
	items := make([]orderpick.Item, len(groupFields))
	for i, f := range groupFields {
		items[i] = ui.OrderByName(f.label, f.name)
	}
	return items
}

// sortItems are the fields the sort modal offers: those of the groups, and
// the time of the run.
func sortItems() []orderpick.Item {
	return append(groupItems(), ui.OrderByTime("Created", createdField))
}

// orderFields are the fields the levels name: those of the groups, and the
// time a run was created.
func orderFields() []feed.Field[core.Run] {
	fields := make([]feed.Field[core.Run], 0, len(groupFields)+1)
	for _, f := range groupFields {
		fields = append(fields, feed.Field[core.Run]{Name: f.name, Key: f.key})
	}
	return append(fields, feed.Field[core.Run]{Name: createdField, Time: createdAt})
}

func createdAt(r core.Run) time.Time { return r.CreatedAt }

// groupsOf returns the groups of the levels, outermost first.
func groupsOf(levels []orderpick.Choice) []feed.Group[core.Run] {
	return feed.Groups(orderFields(), ui.OrderLevels(levels))
}

// sortLess returns the order of the levels: each level's field in turn, a
// run without a value after those with one, and the newest first among those
// the levels tie.
func sortLess(levels []orderpick.Choice) func(a, b core.Run) int {
	return feed.Less(orderFields(), ui.OrderLevels(levels), createdAt)
}

// groupModal returns the modal that picks the levels the runs are grouped by.
func (m *Modal) groupModal() ui.Modal {
	return ui.NewGroupModal(orderTitle, groupItems(), m.levels, m.keys.order, m.opts.icons, m.setGroups)
}

// sortModal returns the modal that picks the fields the runs are sorted by.
// It says in its title that the sort covers the pages loaded: GitHub sorts
// no further.
func (m *Modal) sortModal() ui.Modal {
	return ui.NewSortLevelsModal(orderTitle+" (loaded)", sortItems(), m.sorts, m.keys.order, m.opts.icons, m.setSort)
}

// orderTitle names the runs in the title of the modals that order them.
const orderTitle = "Workflow runs"

// groupFuncs are the groups in force, none when the grouping is off.
func (m *Modal) groupFuncs() []feed.Group[core.Run] {
	if m.off {
		return nil
	}
	return groupsOf(m.levels)
}

// sortFunc is the order in force, nil for GitHub's.
func (m *Modal) sortFunc() func(a, b core.Run) int {
	if len(m.sorts) == 0 || m.sortOff {
		return nil
	}
	return sortLess(m.sorts)
}

// setGroups groups the runs by levels, or flattens them with none. Choosing
// levels switches the grouping on.
func (m *Modal) setGroups(levels []orderpick.Choice) tea.Cmd {
	m.levels = append([]orderpick.Choice(nil), levels...)
	m.off = false
	return m.regroup()
}

// setSort sorts the runs by levels, or back to GitHub's order with none.
// Choosing levels switches the sort on.
func (m *Modal) setSort(levels []orderpick.Choice) tea.Cmd {
	m.sorts = append([]orderpick.Choice(nil), levels...)
	m.sortOff = false
	return m.regroup()
}

// toggleGroups switches the grouping off, and back on. It returns false when
// there are no levels to switch.
func (m *Modal) toggleGroups() (tea.Cmd, bool) {
	if len(m.levels) == 0 {
		return nil, false
	}
	m.off = !m.off
	return m.regroup(), true
}

// toggleSort switches the sort off, and back on. It returns false when there
// is no sort to switch.
func (m *Modal) toggleSort() (tea.Cmd, bool) {
	if len(m.sorts) == 0 {
		return nil, false
	}
	m.sortOff = !m.sortOff
	return m.regroup(), true
}

// regroup lays the runs out by the groups and the sort in force. The feed
// keeps the cursor on its run, so the jobs shown stay.
func (m *Modal) regroup() tea.Cmd {
	return tea.Batch(m.runs.SetGroups(m.groupFuncs()), m.runs.SetSort(m.sortFunc()))
}
