package tui

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	historysvc "github.com/eggzec/gh-tui/internal/service/history"
	"github.com/eggzec/gh-tui/internal/tui/history"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// pagedBranches serves two pages of branches: a long first page, so that the
// cursor is far from its end and nothing reads the second on its own, and a
// last page with one branch.
type pagedBranches struct{ keyHistory }

func (pagedBranches) Branches(_ context.Context, q historysvc.BranchesQuery) (core.Page[core.Branch], error) {
	if q.Cursor != "" {
		return core.Page[core.Branch]{Items: []core.Branch{{Name: "zlast", SHA: "def456"}}}, nil
	}
	items := []core.Branch{{Name: "main", SHA: "abc123"}}
	for i := range 59 {
		items = append(items, core.Branch{Name: fmt.Sprintf("feat-%02d", i), SHA: "abc123"})
	}
	return core.Page[core.Branch]{Items: items, Next: "page2"}, nil
}

// branchPaneApp opens the history with two pages of branches and focuses the
// branches.
func branchPaneApp(t *testing.T) *Model {
	t.Helper()
	m := newKeysAppOpts(t, true, func(*config.Config) {}, WithHistory(history.Opener(pagedBranches{}, config.Default().Keys, history.WithVoice(ui.NewVoice(config.Default().Keys, "")))))
	pressNames(t, m, "B", "shift+tab")
	if _, ok := m.modals.Top().(*history.Modal); !ok {
		t.Fatalf("top is %T, want the history", m.modals.Top())
	}
	return m
}

func pressNames(t *testing.T, m *Model, names ...string) {
	t.Helper()
	for _, name := range names {
		msg, _ := keyPress(name)
		driveKeys(t, m, m.key(msg))
		m.View()
	}
}

func TestHistoryFilterOpensOverAndEscReturns(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := branchPaneApp(t)
		hist := m.modals.Top()
		pressNames(t, m, "f")
		if _, ok := m.modals.Top().(*ui.FilterModal); !ok {
			t.Fatalf("f opened %T, want the filter form", m.modals.Top())
		}
		pressNames(t, m, "esc")
		if m.modals.Top() != hist {
			t.Errorf("esc left %T on top, want the history again", m.modals.Top())
		}
	})
}

func TestHistoryFilterMovesTheCursorToTheBranch(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := branchPaneApp(t)
		hist := m.modals.Top().(*history.Modal)
		pressNames(t, m, "f")
		if _, ok := m.modals.Top().(*ui.FilterModal); !ok {
			t.Fatalf("f opened %T, want the filter form", m.modals.Top())
		}
		// The form opens on the branch under the cursor, so a chosen
		// branch is what moves it.
		if f, _ := hist.Filter(); f.Query != "branch:main" {
			t.Fatalf("the form opens on %q, want the branch under the cursor", f.Query)
		}
		pressNames(t, m, "l", "enter")
		if m.modals.Top() != ui.Modal(hist) {
			t.Fatalf("applying left %T on top, want the history", m.modals.Top())
		}
		if f, _ := hist.Filter(); f.Query != "branch:feat-00" {
			t.Errorf("the cursor is on %q after the form chose the next branch, want feat-00", f.Query)
		}
	})
}

func TestHistoryFilterDroppedWhenHistoryClosesFirst(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := branchPaneApp(t)
		hist := m.modals.Top()
		msg, _ := keyPress("f")
		held := m.key(msg)
		if _, ok := m.modals.Top().(*ui.FilterModal); ok {
			t.Fatal("the form opened before the last page was read")
		}
		pressNames(t, m, "esc")
		if m.modals.Top() == hist {
			t.Fatal("esc didn't close the history")
		}
		driveKeys(t, m, held)
		if _, ok := m.modals.Top().(*ui.FilterModal); ok {
			t.Errorf("the form opened over %T after the history was closed", m.modals.Top())
		}
	})
}
