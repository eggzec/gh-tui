package notifications

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// updatedField is the sort by the time of the last update. GitHub lists the
// threads newest first, so that is its order unless it is flipped.
const updatedField = "updated"

// sortItems are the fields the sort modal offers: the fields the list can be
// grouped by, and the time of the update.
func sortItems() []orderpick.Item {
	items := groupItems()
	return append(items, ui.OrderByTime("Updated", updatedField))
}

// sortLess returns the order of the levels: each level's field in turn, a
// thread without a value after those with one, and the newest update first
// among those the levels tie.
func (s *Section) sortLess(levels []orderpick.Choice) func(a, b core.Notification) int {
	type level struct {
		field groupField
		desc  bool
		time  bool
	}
	var ls []level
	for _, c := range levels {
		name, _ := c.Value.(string)
		if name == updatedField {
			ls = append(ls, level{desc: c.Desc, time: true})
			continue
		}
		for _, f := range groupFields {
			if f.name == name {
				ls = append(ls, level{field: f, desc: c.Desc})
			}
		}
	}
	return func(a, b core.Notification) int {
		for _, l := range ls {
			var d int
			if l.time {
				d = b.UpdatedAt.Compare(a.UpdatedAt)
			} else {
				ka, kb := l.field.key(s, a), l.field.key(s, b)
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
		return b.UpdatedAt.Compare(a.UpdatedAt)
	}
}

// sortModal returns the modal that picks the fields the list is sorted by.
func (s *Section) sortModal() ui.Modal {
	return ui.NewSortLevelsModal(s.Title(), sortItems(), s.sorts, s.keys.order, s.icons, s.setSort)
}

// setSort sorts the list by levels, or back to GitHub's order with none.
// Choosing levels switches the sort on.
func (s *Section) setSort(levels []orderpick.Choice) tea.Cmd {
	s.sorts = append([]orderpick.Choice(nil), levels...)
	s.sortOff = false
	cmd := s.feed.SetSort(s.sortFunc())
	s.renderHeader()
	return cmd
}

// sortFunc is the order in force, nil for GitHub's.
func (s *Section) sortFunc() func(a, b core.Notification) int {
	if len(s.sorts) == 0 || s.sortOff {
		return nil
	}
	return s.sortLess(s.sorts)
}

// toggleSort switches the sort off, and back on. It returns false when there
// is no sort to switch.
func (s *Section) toggleSort() (tea.Cmd, bool) {
	if len(s.sorts) == 0 {
		return nil, false
	}
	s.sortOff = !s.sortOff
	cmd := s.feed.SetSort(s.sortFunc())
	s.renderHeader()
	return cmd, true
}

// sortChip words the sort for the header, such as "sort repository, updated".
func (s *Section) sortChip() string {
	names := make([]string, len(s.sorts))
	for i, c := range s.sorts {
		names[i], _ = c.Value.(string)
	}
	return "sort " + strings.Join(names, ", ")
}
