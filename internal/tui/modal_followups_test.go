package tui

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/modals"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// topList is a content modal with a list of its own to filter and sort, as
// the Actions modal has.
type topList struct {
	fakeModal
	applied []filterform.AppliedMsg
}

func init() { fakePriorities[reflect.TypeFor[*topList]()] = modals.PriorityContent }

func (l *topList) Filter() (ui.Filter, bool) {
	return ui.Filter{Spec: filterform.Spec{
		Fields: []filterform.Field{{
			Key: "state", Label: "State", Kind: filterform.Choice, Qualifier: "is",
			Options: []filterform.Item{{Label: "Open", Value: "open"}, {Label: "Closed", Value: "closed"}},
			Default: filterform.TextValue("open"),
		}},
		Sort: &filterform.SortField{
			Options: []filterform.SortOption{ui.SortByTime("Updated", "updated")},
			Default: filterform.Sort{By: "updated", Desc: true},
		},
	}, Subject: "runs"}, true
}

func (l *topList) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	l.applied = append(l.applied, msg)
	return nil
}

// openedTitle returns the title of the filter or sort modal on top.
func openedTitle(t *testing.T, m *Model) string {
	t.Helper()
	switch mod := m.modals.Top().(type) {
	case *ui.FilterModal:
		return mod.Title()
	case *ui.SortModal:
		return mod.Title()
	default:
		t.Fatalf("modal = %T, want the filter or sort modal", mod)
		return ""
	}
}

// An open composer with no text keeps no draft when the app quits.
func TestCtrlCKeepsNoDraftOfABlankComposer(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		d := &countDrafts{}
		m := newKeysAppPulls(t, true, func(*config.Config) {}, keyInbox{}, []pulls.Option{pulls.WithDrafts(d, "test")})
		driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 1} })
		driveKeys(t, m, m.key(press("c")))
		if n := d.saves.Load(); n != 0 {
			t.Fatalf("opening the composer saved %d drafts", n)
		}
		cmd := m.key(ctrlC)
		if n := d.saves.Load(); n != 0 {
			t.Errorf("a blank composer was saved %d times, want none", n)
		}
		if !quits(cmd) {
			t.Error("ctrl+c didn't quit")
		}
	})
}

// A filter or sort message finds the list of the top modal when it has one,
// and the filter modal opens over it and returns to it.
func TestRootFilterFindsTheTopModalsList(t *testing.T) {
	t.Parallel()
	for name, msg := range map[string]tea.Msg{"filter": ui.OpenFilterMsg{}, "sort": ui.OpenSortMsg{}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, pulls, _ := newFilterApp(t)
			top := &topList{}
			top.title = "Actions"
			run(m, ui.OpenModalOver(top, nil))
			run(m, func() tea.Msg { return msg })
			title := openedTitle(t, m)
			if !strings.Contains(title, "Actions") || strings.Contains(title, "Pull requests") {
				t.Errorf("title = %q, want the filter of the top modal", title)
			}
			run(m, m.key(press("esc")))
			if m.modals.Top() != top {
				t.Errorf("esc left %v, want the modal the filter opened over", m.modals.Top())
			}
			if len(pulls.applied) != 0 {
				t.Errorf("the section applied %d filters", len(pulls.applied))
			}
		})
	}
}

// With no modal open, or a top modal that has no list, a filter or sort
// message still acts on the focused section, as before.
func TestRootFilterFallsBackToTheSection(t *testing.T) {
	t.Parallel()
	for name, msg := range map[string]tea.Msg{"filter": ui.OpenFilterMsg{}, "sort": ui.OpenSortMsg{}} {
		for _, over := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s over a modal %v", name, over), func(t *testing.T) {
				t.Parallel()
				m, pulls, _ := newFilterApp(t)
				pulls.sorts = true
				if over {
					plain := &fakeModal{title: "Plain"}
					run(m, ui.OpenModalOver(plain, nil))
					if s, host := m.hosted(); s != nil || host != nil {
						t.Fatalf("hosted() = %v, %v over a modal with no list, want none", s, host)
					}
				}
				if s, host := m.hosted(); s != nil || host != nil {
					t.Fatalf("hosted() = %v, %v, want none", s, host)
				}
				run(m, func() tea.Msg { return msg })
				if title := openedTitle(t, m); !strings.Contains(title, "Pull requests") {
					t.Errorf("title = %q, want the filter of the section", title)
				}
			})
		}
	}
}

// yieldsQuit reports whether running cmd, and the commands of a batch it returns,
// yields tea.QuitMsg.
func yieldsQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		return slices.ContainsFunc(msg, quits)
	}
	return false
}

// :q keeps an issue modal with an unsent comment open, with a toast, while
// ctrl+c still quits and an empty prompt lets :q quit.
func TestQuitKeepsAnIssueWithAnUnsentComment(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 1} })
		first := m.modals.Top()
		driveKeys(t, m, m.key(press("c")))
		for _, r := range "hi" {
			driveKeys(t, m, m.key(press(string(r))))
		}
		cmd := m.quit(nil)
		if m.modals.Top() != first {
			t.Fatalf(":q left %v, want the issue with the comment on top", m.modals.Top())
		}
		if !hasToast(m, "Send or drop the comment first.") {
			t.Errorf("toast = %q, want the refusal", toasted(m))
		}
		if yieldsQuit(cmd) {
			t.Error(":q quit with a comment unsent")
		}
		if !yieldsQuit(m.quitNow()) {
			t.Error("ctrl+c did not quit with a comment unsent")
		}
	})
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 1} })
		driveKeys(t, m, m.key(press("c")))
		if !yieldsQuit(m.quit(nil)) {
			t.Error(":q with an empty prompt did not quit")
		}
	})
}

// An issue modal with a comment being written is not closed by
// the app on its own account: esc on the modal opened over it keeps it, with a
// toast, until the comment is sent or dropped.
func TestEscRunKeepsAnIssueWithAnUnsentComment(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, true)
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 1} })
		first := m.modals.Top()
		driveKeys(t, m, m.key(press("c")))
		for _, r := range "hi" {
			driveKeys(t, m, m.key(press(string(r))))
		}
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 2} })
		second := m.modals.Top()
		if second == first {
			t.Fatal("the second issue did not open")
		}
		driveKeys(t, m, m.key(press("esc")))
		if m.modals.Top() != first {
			t.Fatalf("esc left %v, want the issue with the comment on top", m.modals.Top())
		}
		if !hasToast(m, "Send or drop the comment first.") {
			t.Errorf("toast = %q, want the refusal", toasted(m))
		}
		// Esc in the prompt drops the comment, and the next esc closes.
		driveKeys(t, m, m.key(press("esc")))
		if m.modals.Top() != first {
			t.Fatalf("dropping the comment left %v", m.modals.Top())
		}
		driveKeys(t, m, m.key(press("esc")))
		if m.modals.Top() != nil {
			t.Errorf("esc after the comment was dropped left %v open", m.modals.Top())
		}
	})
}
