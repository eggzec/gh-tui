package notifications

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// groupField is a way to group the threads: what a thread is filed under, and
// the words that head the group.
type groupField struct {
	name  string
	label string
	// key is the group a thread is in, "" for none.
	key func(s *Section, n core.Notification) string
	// head words the group of key.
	head func(key string) string
}

// groupFields are the ways to group the threads, in the order the modal lists
// them.
var groupFields = []groupField{
	{"repository", "Repository", func(_ *Section, n core.Notification) string { return n.Repo.String() }, nil},
	{"owner", "Owner", func(_ *Section, n core.Notification) string { return n.Repo.Owner }, nil},
	{"type", "Type", func(_ *Section, n core.Notification) string { return typeName(n.Subject.Type) }, typeHead},
	{"state", "State", func(s *Section, n core.Notification) string {
		st, known := s.State(n)
		if !known {
			return ""
		}
		if names := stateNames(n.Subject.Type, st); len(names) > 0 {
			return names[0]
		}
		return ""
	}, stateHead},
	{"reason", "Reason", func(_ *Section, n core.Notification) string { return n.Reason }, shortReason},
	{"unread", "Unread", func(_ *Section, n core.Notification) string {
		if n.Unread {
			return "unread"
		}
		return "read"
	}, nil},
}

func typeHead(key string) string {
	for _, t := range types {
		if t.name == key {
			return t.label
		}
	}
	return key
}

func stateHead(key string) string {
	for _, it := range stateItems {
		if it.Value == key {
			return it.Label
		}
	}
	return key
}

// groupsOf returns the groups of the levels, outermost first.
func (s *Section) groupsOf(levels []orderpick.Choice) []feed.Group[core.Notification] {
	var out []feed.Group[core.Notification]
	for _, c := range levels {
		name, _ := c.Value.(string)
		for _, f := range groupFields {
			if f.name != name {
				continue
			}
			out = append(out, feed.Group[core.Notification]{
				Key:   func(n core.Notification) string { return f.key(s, n) },
				Label: f.head,
				Desc:  c.Desc,
			})
		}
	}
	return out
}

// groupItems are the fields the modal offers.
func groupItems() []orderpick.Item {
	items := make([]orderpick.Item, len(groupFields))
	for i, f := range groupFields {
		items[i] = ui.OrderByName(f.label, f.name)
	}
	return items
}

// groupedByState reports whether the list is grouped, and by state at some level.
func (s *Section) groupedByState() bool {
	if s.off {
		return false
	}
	for _, c := range s.levels {
		if c.Value == "state" {
			return true
		}
	}
	return false
}

// groupModal returns the modal that picks the levels the list is grouped by.
func (s *Section) groupModal() ui.Modal {
	return ui.NewGroupModal(s.Title(), groupItems(), s.levels, s.keys.order, s.icons, s.setGroups)
}

// setGroups groups the list by levels, or flattens it with none. Choosing
// levels switches the grouping on.
func (s *Section) setGroups(levels []orderpick.Choice) tea.Cmd {
	s.levels = append([]orderpick.Choice(nil), levels...)
	s.off = false
	cmd := s.feed.SetGroups(s.groupsOf(s.levels))
	s.renderHeader()
	return cmd
}

// toggleGroups switches the grouping off, and back on. It returns false when
// there are no levels to switch.
func (s *Section) toggleGroups() (tea.Cmd, bool) {
	if len(s.levels) == 0 {
		return nil, false
	}
	var groups []feed.Group[core.Notification]
	s.off = !s.off
	if !s.off {
		groups = s.groupsOf(s.levels)
	}
	cmd := s.feed.SetGroups(groups)
	s.renderHeader()
	return cmd, true
}
