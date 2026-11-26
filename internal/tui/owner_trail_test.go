package tui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/modals"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// authorModal stands for the pull request and issue modals, which show the
// owner page of their author over themselves.
type authorModal struct {
	fakeModal
	author string
}

func (a *authorModal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActQuit, ui.ActDismiss:
		return ui.CloseModal(a), true
	case "owner":
		return ui.ShowOwner(a.author), true
	}
	return nil, false
}

// KeyLayers are those of a modal that takes no typing, so the owner key
// reaches Act.
func (a *authorModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{{Source: "preview", Context: "preview"}}
}

func init() {
	fakePriorities[reflect.TypeFor[*authorModal]()] = modals.PriorityContent
}

// TestOwnerFromPullBack checks that the owner page opened from a modal
// stays over it, so that backspace returns to the modal.
func TestOwnerFromPullBack(t *testing.T) {
	t.Parallel()
	m, _ := newOwnerApp(t, newFakeOwners())
	pr := &authorModal{title: "Pull request", author: "octocat"}
	drive(m, ui.OpenModal(pr))
	drive(m, m.key(press("@")))
	if top := ownerOn(m); top == nil || top.login != "octocat" {
		t.Fatalf("on top %v, want octocat's page", m.modals.Top())
	}
	drive(m, m.key(press("backspace")))
	if m.modals.Top() != pr {
		t.Errorf("on top %v after backspace, want the pull request modal", m.modals.Top())
	}
}

// The owner key of a modal whose author is the viewer shows the dashboard
// with no modal over it, from the dashboard and from a repository.
func TestOwnerFromOwnModal(t *testing.T) {
	t.Parallel()
	for _, repo := range []bool{false, true} {
		name := "dashboard"
		if repo {
			name = "repository"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, _ := newOwnerApp(t, newFakeOwners(), WithLogin("mona"))
			if repo {
				runCommand(t, m, "goto eggzec/gh-tui")
			}
			drive(m, ui.OpenModal(&authorModal{title: "Issue", author: "mona"}))
			drive(m, m.key(press("@")))
			if m.modals.Top() != nil || m.screen != dashScreen {
				t.Errorf("on top %v, screen %d; want the dashboard alone", m.modals.Top(), m.screen)
			}
		})
	}
}
