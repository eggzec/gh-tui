package notifications

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// Modal shows the notifications section in a modal, over whatever is open.
type Modal struct {
	s *Section
}

var (
	_ ui.Modal  = (*Modal)(nil)
	_ ui.Actor  = (*Modal)(nil)
	_ ui.Hider  = (*Modal)(nil)
	_ ui.Hosted = (*Modal)(nil)
)

// NewModal returns a modal that shows s, which it focuses.
func NewModal(s *Section) *Modal {
	s.Focus()
	return &Modal{s: s}
}

// Section returns the section the modal shows.
func (m *Modal) Section() *Section { return m.s }

// Title returns the section's title.
func (m *Modal) Title() string { return m.s.Title() }

// KeyContext returns the key context of the notifications.
func (m *Modal) KeyContext() string { return "notifications" }

// SetSize sizes the section to the modal.
func (m *Modal) SetSize(width, height int) { m.s.SetSize(width, height) }

// SetTheme restyles the section.
func (m *Modal) SetTheme(t ui.Theme) { m.s.SetTheme(t) }

// View draws the section.
func (m *Modal) View() string { return m.s.View() }

// Commands lists the commands the section takes.
func (m *Modal) Commands() []string { return nil }

// KeyLayers returns the section's key layers.
func (m *Modal) KeyLayers() []keyhelp.Layer { return m.s.KeyLayers() }

// Capturing reports whether the section takes every key.
func (m *Modal) Capturing() bool { return m.s.Capturing() }

// Hide stops what the section reads ahead, as when it is blurred.
func (m *Modal) Hide() { m.s.Blur() }

// Update passes msg to the section. The modals it opens go over this one, so
// that closing them returns here.
func (m *Modal) Update(msg tea.Msg) tea.Cmd {
	if !m.s.feed.Focused() {
		m.s.Focus()
	}
	return m.over(m.s.Update(msg))
}

// over makes the modals that cmd opens open over the modal.
func (m *Modal) over(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case ui.OpenModalMsg:
			if msg.Back == nil {
				msg.Back = m
			}
			return msg
		case tea.BatchMsg:
			for i, c := range msg {
				msg[i] = m.over(c)
			}
			return msg
		default:
			return msg
		}
	}
}

// Act takes the intents of the global keys: the section clears what the user
// asked to see first, and then the modal closes, returning to what was under
// it.
func (m *Modal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActDismiss:
		if cmd, ok := m.s.feed.ClearTransient(); ok {
			return cmd, true
		}
		return ui.LeaveModal(m), true
	case ui.ActQuit, ui.ActBack, "notifications":
		return ui.LeaveModal(m), true
	}
	return nil, false
}
