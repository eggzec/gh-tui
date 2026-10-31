package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// groupFixture opens the group modal over two fields, with chosen in force,
// and returns what apply was given.
func groupFixture(chosen ...orderpick.Choice) (m *OrderModal, applied *[][]orderpick.Choice) {
	d := config.Default()
	applied = new([][]orderpick.Choice)
	fields := []orderpick.Item{{Value: "type", Title: "Type"}, {Value: "reason", Title: "Reason"}}
	m = NewGroupModal("Notifications", fields, chosen, OrderPickKeys(d.Keys), NewIcons(d.UI.Icons), func(c []orderpick.Choice) tea.Cmd {
		*applied = append(*applied, c)
		return nil
	})
	m.SetSize(120, 30)
	return m, applied
}

func sendGroup(m *OrderModal, keys ...string) {
	for _, k := range keys {
		for _, msg := range drain(m.Update(keyOf(k))) {
			m.Update(msg)
		}
	}
}

func TestGroupModalTitleAndEmptyText(t *testing.T) {
	m, _ := groupFixture()
	if got, want := m.Title(), "Group"+NewIcons(config.Default().UI.Icons).Separator+"Notifications"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Nothing: a flat list") {
		t.Errorf("no empty text:\n%s", view)
	}
}

func TestGroupModalApplyingNoneFlattens(t *testing.T) {
	m, applied := groupFixture()
	sendGroup(m, "enter")
	if len(*applied) != 1 || len((*applied)[0]) != 0 {
		t.Errorf("applied %v, want one call with none", *applied)
	}
}

func TestGroupModalEscapeDoesNotApply(t *testing.T) {
	m, applied := groupFixture(orderpick.Choice{Value: "type"})
	sendGroup(m, "esc")
	if len(*applied) != 0 {
		t.Errorf("esc applied %v, want nothing", *applied)
	}
}
