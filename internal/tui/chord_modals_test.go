package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// noChordWait keeps the app from waiting on a real timer for the second g.
func noChordWait(m *Model) {
	m.after = func(time.Duration, tea.Msg) tea.Cmd { return nil }
}

// "g g" reaches the top of the filter modal's fields.
func TestChordTopInTheFilterModal(t *testing.T) {
	t.Parallel()
	m, pulls, _ := newFilterApp(t)
	noChordWait(m)
	pulls.labels = []filterform.Item{{Label: "bug", Value: "bug"}}
	for _, k := range []string{"f", "j", "g", "g", "space", "k", "enter"} {
		run(m, m.key(press(k)))
	}
	if got, want := filterModal(t, m).Query(), "is:open"; got != want {
		t.Errorf("query = %q, want %q: the State field was not reached", got, want)
	}
}

// "g g" reaches the top of the sort modal's list.
func TestChordTopInTheSortModal(t *testing.T) {
	t.Parallel()
	m, pulls, _ := newFilterApp(t)
	noChordWait(m)
	pulls.sorts = true
	for _, k := range []string{"s", "j", "g", "g", "enter"} {
		run(m, m.key(press(k)))
	}
	if len(pulls.applied) != 1 {
		t.Fatalf("applied %+v, want one query", pulls.applied)
	}
	if got, want := pulls.applied[0].Query, "is:closed sort:updated-desc"; got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
}

// "g g" reaches the top of the group modal's fields.
func TestChordTopInTheGroupModal(t *testing.T) {
	t.Parallel()
	m, _, _ := newFilterApp(t)
	noChordWait(m)
	d := config.Default()
	var applied [][]orderpick.Choice
	fields := []orderpick.Item{{Value: "a", Title: "A"}, {Value: "b", Title: "B"}, {Value: "c", Title: "C"}}
	mod := ui.NewGroupModal("Things", fields, nil, ui.OrderPickKeys(d.Keys), ui.NewIcons(d.UI.Icons), func(c []orderpick.Choice) tea.Cmd {
		applied = append(applied, c)
		return nil
	})
	run(m, ui.OpenModal(mod))
	for _, k := range []string{"j", "j", "g", "g", "space", "enter"} {
		run(m, m.key(press(k)))
	}
	if len(applied) != 1 || len(applied[0]) != 1 || applied[0][0].Value != "a" {
		t.Errorf("applied %+v, want the first field", applied)
	}
}
