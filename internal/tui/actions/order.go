package actions

import (
	"strings"

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
		items[i] = orderpick.Item{Value: f.name, Title: f.label, AscLabel: "A to Z", DescLabel: "Z to A"}
	}
	return items
}

// sortItems are the fields the sort modal offers: those of the groups, and
// the time of the run.
func sortItems() []orderpick.Item {
	return append(groupItems(), orderpick.Item{Value: createdField, Title: "Created", AscLabel: "Newest first", DescLabel: "Oldest first"})
}

// groupsOf returns the groups of the levels, outermost first.
func groupsOf(levels []orderpick.Choice) []feed.Group[core.Run] {
	var out []feed.Group[core.Run]
	for _, c := range levels {
		name, _ := c.Value.(string)
		for _, f := range groupFields {
			if f.name == name {
				out = append(out, feed.Group[core.Run]{Key: f.key, Desc: c.Desc})
			}
		}
	}
	return out
}

// sortLess returns the order of the levels: each level's field in turn, a
// run without a value after those with one, and the newest first among those
// the levels tie.
func sortLess(levels []orderpick.Choice) func(a, b core.Run) int {
	type level struct {
		field groupField
		desc  bool
		time  bool
	}
	var ls []level
	for _, c := range levels {
		name, _ := c.Value.(string)
		if name == createdField {
			ls = append(ls, level{desc: c.Desc, time: true})
			continue
		}
		for _, f := range groupFields {
			if f.name == name {
				ls = append(ls, level{field: f, desc: c.Desc})
			}
		}
	}
	return func(a, b core.Run) int {
		for _, l := range ls {
			var d int
			if l.time {
				d = b.CreatedAt.Compare(a.CreatedAt)
			} else {
				ka, kb := l.field.key(a), l.field.key(b)
				switch {
				case ka == kb:
				case ka == "":
					return 1
				case kb == "":
					return -1
				default:
					d = strings.Compare(ka, kb)
				}
			}
			if l.desc {
				d = -d
			}
			if d != 0 {
				return d
			}
		}
		return b.CreatedAt.Compare(a.CreatedAt)
	}
}

// groupModal returns the modal that picks the levels the runs are grouped by.
func (m *Modal) groupModal() ui.Modal {
	return ui.NewGroupModal(m.section(), groupItems(), m.levels, m.keys.order, m.opts.icons, m.setGroups)
}

// sortModal returns the modal that picks the fields the runs are sorted by.
// It says in its title that the sort covers the pages loaded: GitHub sorts
// no further.
func (m *Modal) sortModal() ui.Modal {
	return ui.NewSortLevelsModal(m.section()+" (loaded)", sortItems(), m.sorts, m.keys.order, m.opts.icons, m.setSort)
}

// section names the runs in the title of the modals that order them.
func (m *Modal) section() string { return "Workflow runs" }

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
