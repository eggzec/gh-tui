package owner

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Modal is the page of one user or organization in a modal, over the screen
// or the modal that opened it. Create one with [NewModal].
type Modal struct {
	*Section
	// start reads the page, and is returned by Init.
	start tea.Cmd
}

var (
	_ ui.Modal = (*Modal)(nil)
	_ ui.Actor = (*Modal)(nil)
)

// NewModal returns the modal of the account login, which shows s. Call Init
// to read the page once the modal is open.
func NewModal(s *Section, login string) *Modal {
	m := &Modal{Section: s}
	s.Focus()
	s.started = true
	s.leave = func() tea.Cmd { return ui.CloseModal(m) }
	m.start = s.Update(ui.OwnerMsg{Login: login})
	return m
}

// Title returns the login of the account on view.
func (m *Modal) Title() string {
	if login := m.Login(); login != "" {
		return login
	}
	return m.Section.Title()
}

// Discard implements ui.Discarder: the page is gone, so its requests in
// flight stop.
func (m *Modal) Discard() { m.cancel() }

// Init reads the page on view.
func (m *Modal) Init() tea.Cmd { return m.start }

// Commands implements ui.Commanded: the commands that act on the page are the
// ones of the lists' selection.
func (m *Modal) Commands() []string { return []string{ui.CommandCopy} }

// Act implements ui.Actor. The quit and dismiss keys close the modal. The
// back key drops a zoom, then goes back through the pages the modal showed,
// and otherwise leaves the app to return to the modal this one replaced. The
// owner key opens the page of the account under the cursor over this one,
// and the repo key closes the modal, then shows the repository under the
// cursor. Every other intent is the app's to refuse while it is
// open.
func (m *Modal) Act(action string) (tea.Cmd, bool) {
	if m.Capturing() {
		return nil, false
	}
	switch action {
	case ui.ActQuit, ui.ActDismiss:
		return ui.CloseModal(m), true
	case ui.ActBack:
		if m.undoZoom() {
			return nil, true
		}
		// A person row opens a modal of its own, so the pages never stack
		// here: back returns to the modal this one replaced.
		return nil, false
	case "owner":
		sel, ok := m.Selected()
		if !ok || sel.Owner == "" || strings.EqualFold(sel.Owner, m.Login()) {
			return nil, true
		}
		// The page opened over this one replaces it at the same rank, so back
		// returns here, and the modal held under this page stays held.
		return ui.ShowOwner(sel.Owner), true
	case "repo":
		sel, ok := m.Selected()
		if !ok || sel.Repo.Name == "" {
			return nil, true
		}
		return m.selectRepo(sel.Repo), true
	}
	return nil, false
}

// undoZoom drops a zoom the user asked for, and reports whether there was
// one.
func (s *Section) undoZoom() bool {
	if s.page == nil {
		return false
	}
	s.syncNav()
	if !s.ctl.UndoZoom() {
		return false
	}
	s.layout()
	s.applyNav()
	s.render()
	return true
}
