package tui

import (
	"log/slog"
	"reflect"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/actions"
	"github.com/eggzec/gh-tui/internal/tui/files"
	"github.com/eggzec/gh-tui/internal/tui/history"
	"github.com/eggzec/gh-tui/internal/tui/issues"
	"github.com/eggzec/gh-tui/internal/tui/modals"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/releases"
	"github.com/eggzec/gh-tui/internal/tui/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// maxHeldModals is how many modals the manager keeps open under the top one.
const maxHeldModals = 10

// modalPriorities ranks every modal by its type. The numbers live here only:
// a higher one opens over a lower one, an equal one replaces it, and a lower
// one is refused. Keys are the type each constructor returns, a pointer.
var modalPriorities = map[reflect.Type]modals.Priority{
	reflect.TypeFor[*search.Modal]():        modals.PriorityAnywhere,
	reflect.TypeFor[*notifications.Modal](): modals.PriorityAnywhere,

	reflect.TypeFor[*files.Finder](): modals.PriorityList,

	reflect.TypeFor[*pulls.Panes]():    modals.PriorityContent,
	reflect.TypeFor[*issues.Detail]():  modals.PriorityContent,
	reflect.TypeFor[*releases.Modal](): modals.PriorityContent,
	reflect.TypeFor[*history.Modal]():  modals.PriorityContent,
	reflect.TypeFor[*actions.Modal]():  modals.PriorityContent,

	reflect.TypeFor[*files.Preview](): modals.PriorityDetail,
	reflect.TypeFor[*actions.Log]():   modals.PriorityDetail,
	reflect.TypeFor[*textModal]():     modals.PriorityDetail,

	reflect.TypeFor[*ui.FilterModal](): modals.PriorityShape,
	reflect.TypeFor[*ui.OrderModal]():  modals.PriorityShape,
	reflect.TypeFor[*menuModal]():      modals.PriorityShape,

	reflect.TypeFor[*pulls.MergeDialog](): modals.PriorityForm,

	reflect.TypeFor[*ui.ConfirmModal](): modals.PriorityConfirm,

	reflect.TypeFor[*authModal](): modals.PriorityPrompt,
}

// priorityOf returns the rank of mod's type, and false for a type the table
// does not list.
func (m *Model) priorityOf(mod ui.Modal) (modals.Priority, bool) {
	p, ok := m.priorities[reflect.TypeOf(mod)]
	return p, ok
}

// guardOf returns the guard of mod: a modal that holds text the user wrote
// saves it when dismissed, and when the app quits it closes without asking.
// An issue modal, which can keep no draft, stays open instead of losing a
// comment being written.
func (m *Model) guardOf(mod ui.Modal) modals.Guard {
	if d, ok := mod.(*issues.Detail); ok {
		// The comment prompt keeps no draft, so the modal stays until the
		// comment is sent or dropped.
		return func(_ modals.Reason) modals.Closing {
			// :q asks too, so it stays open on a draft; ctrl+c quits
			// without asking (Model.quitNow) and must always quit.
			if d.HasDraft() {
				return modals.Refuse("Send or drop the comment first.")
			}
			return modals.Close()
		}
	}
	s, ok := mod.(ui.Saver)
	if !ok {
		return nil
	}
	return func(r modals.Reason) modals.Closing {
		if r == modals.Quit {
			return modals.Close()
		}
		return modals.CloseAfter(func() tea.Msg {
			if err := s.Save(); err != nil {
				slog.Warn("save modal", "span", "tui", "err", err)
			}
			return nil
		})
	}
}

// refuseUnranked reports whether mod's type has no rank, which is a bug: the
// open is refused, and the user and the log are told.
func (m *Model) refuseUnranked(mod ui.Modal) (tea.Cmd, bool) {
	if _, ok := m.priorityOf(mod); ok {
		return nil, false
	}
	slog.Warn("modal has no priority", "span", "tui", "type", reflect.TypeOf(mod).String())
	return m.toast.Push(toast.Warning, "This view cannot be opened here."), true
}

// open opens mod by its rank, over the modal over if it is still the open
// one, and returns what the manager asks the root to run. Whether mod is on
// top afterwards is for the caller to check with m.modals.Top().
func (m *Model) open(mod, over ui.Modal) tea.Cmd {
	if mod == nil {
		return nil
	}
	if cmd, refused := m.refuseUnranked(mod); refused {
		return cmd
	}
	prio, _ := m.priorityOf(mod)
	m.cancelGoto()
	if m.line.Focused() {
		m.line.Blur()
		m.lineDone()
	}
	// The help lists the keys of what had them before.
	m.keyhelp.Blur()
	opts := []modals.Option{modals.WithMaximized(m.opensMaximized(mod))}
	if over != nil {
		opts = append(opts, modals.WithOver(over))
	}
	if g := m.guardOf(mod); g != nil {
		opts = append(opts, modals.WithGuard(g))
	}
	return m.apply(m.modals.Add(mod, prio, opts...))
}

// apply turns what the manager did into the toast and the commands the root
// shows. A stale or same open is silent.
func (m *Model) apply(ch modals.Change) tea.Cmd {
	cmds := []tea.Cmd{ch.Cmd}
	switch ch.Refused {
	case modals.NotRefused, modals.Stale, modals.Same:
	case modals.Lower:
		cmds = append(cmds, m.toast.Push(toast.Warning, "Close "+m.modalName(ch.Named)+" first."))
	case modals.Kept:
		if ch.Toast != "" {
			cmds = append(cmds, m.toast.Push(toast.Warning, ch.Toast))
		}
	}
	if ch.Opened || ch.Closed {
		m.resized = true
	}
	return tea.Batch(cmds...)
}
