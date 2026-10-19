package pulls

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

var _ ui.Actor = (*panesModal)(nil)

// Act implements ui.Actor. The quit key closes the modal from anywhere. With
// an overlay open, the keys are its: back and dismiss close a viewer, clearing
// what is transient first, and reach any other overlay as keys, which close
// themselves, as they do an overlay that types; the owner key waits. The
// maximize key flips a viewer. Without an overlay, dismiss closes the modal
// and the owner key shows the page of the author. Every other intent is the app's to refuse while the modal is open.
func (m *panesModal) Act(action string) (tea.Cmd, bool) {
	if action == ui.ActQuit {
		return m.close(), true
	}
	o := m.top()
	if o == nil {
		if m.filesDiffFocused() && m.files.pv.o.typing() {
			// The search of the diff beside the tree takes the keys as text.
			return nil, false
		}
		switch action {
		case ui.ActDismiss:
			if m.clearFilesDiff() {
				return nil, true
			}
			return m.close(), true
		case "owner":
			return m.author(), true
		}
		return nil, false
	}
	if t, ok := o.(interface{ typing() bool }); ok && t.typing() {
		// Its own keys close it, and the other keys are its text.
		return nil, false
	}
	viewer := o.kind() == panes.Viewer
	switch action {
	case ui.ActBack:
		if viewer {
			m.pop()
		} else if c, ok := o.(interface{ cancel() }); ok {
			// The app's back must not act while the picker is open, so the
			// picker is told, since the key never reaches it.
			c.cancel()
			m.settle()
		}
		return nil, true
	case ui.ActDismiss:
		if !viewer {
			return nil, false
		}
		if !o.clear() {
			m.pop()
		}
		return nil, true
	case ui.ActMaximize:
		if !viewer {
			return nil, false
		}
		top := &m.overlays[len(m.overlays)-1]
		top.maximized = !top.maximized
		return nil, true
	case "owner":
		return nil, true
	}
	return nil, false
}

// author closes the modal and shows the page of the author of the pull
// request, which shows in place of the screen behind it.
func (m *panesModal) author() tea.Cmd {
	if ui.Author(m.detail.Author) == "" {
		return nil
	}
	return tea.Sequence(m.close(), ui.ShowOwner(m.detail.Author.Login))
}
