package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// avatarsWait is how long the views wait to draw avatars that arrived,
// so that the avatars of a thread, which arrive one after the other, are
// drawn together rather than each with a drawing of every view.
const avatarsWait = 50 * time.Millisecond

// quitFrame is how long quitting waits after the images were deleted, so
// that the deletes reach the terminal while it still shows the alternate
// screen, where the images are: the program writes what it was sent at
// each frame, and leaves the screen as it quits without writing more.
const quitFrame = 50 * time.Millisecond

// avatarsDueMsg ends the wait to draw the avatars that arrived.
type avatarsDueMsg struct{}

// imagesClearMsg takes the place of the program's quit, or of its
// interrupt, while the terminal holds images, so that they are deleted
// first.
type imagesClearMsg struct{ interrupt bool }

// imagesClearedMsg ends the frame the deletes of the images were given,
// and quits, or interrupts, as the program was asked to.
type imagesClearedMsg struct{ interrupt bool }

// WithAvatars draws avatars with a, which the sections that show them
// share. The app tells it what the terminal shows, fetches what the
// sections drew after each update, and has the sections draw again when
// avatars arrive.
func WithAvatars(a *ui.Avatars) Option {
	return func(m *Model) { m.avatars = a }
}

// loadAvatars ends an update: it fetches the avatars drawn in it, and
// sends those waiting to be. An avatar that took the ID of another has
// the views draw at once, and the avatars those ask for are fetched too.
func (m *Model) loadAvatars() tea.Cmd {
	m.avatars.SetOffline(!m.offSince.IsZero())
	cmd, redraw := m.avatars.Load()
	if redraw != ui.RedrawNow {
		return tea.Batch(cmd, m.redrawAvatars(redraw))
	}
	drawn := m.avatarsChanged()
	again, redraw := m.avatars.Load()
	if redraw == ui.RedrawNow {
		redraw = ui.RedrawSoon
	}
	return tea.Batch(cmd, drawn, again, m.redrawAvatars(redraw))
}

// redrawAvatars has the views draw the avatars when r says.
func (m *Model) redrawAvatars(r ui.Redraw) tea.Cmd {
	switch r {
	case ui.RedrawNow:
		m.avatarsDue = false
		return m.avatarsChanged()
	case ui.RedrawSoon:
		if m.avatarsDue {
			return nil
		}
		m.avatarsDue = true
		return m.after(avatarsWait, avatarsDueMsg{})
	case ui.RedrawNone:
	}
	return nil
}

// avatarsChanged has the header, the sections and the open modal draw
// their avatars again.
func (m *Model) avatarsChanged() tea.Cmd {
	m.drawHeader()
	return m.broadcast(ui.AvatarsMsg{})
}

// setGraphics tells the avatars what the terminal shows, and has the
// sections draw them again if that changed whether they are drawn.
func (m *Model) setGraphics() tea.Cmd {
	if !m.avatars.SetGraphics(m.graphics) {
		return nil
	}
	return m.avatarsChanged()
}

// Filter is the program's message filter: while the terminal holds
// images, it turns the first quit or interrupt, whichever way it came,
// into a message that deletes them and quits a frame later. Until then
// it drops every other quit, which would end the program before the
// deletes were written, since the program writes what it was sent only
// at a frame.
func (m *Model) Filter(_ tea.Model, msg tea.Msg) tea.Msg {
	var interrupt bool
	switch msg.(type) {
	case tea.QuitMsg:
	case tea.InterruptMsg:
		interrupt = true
	default:
		return msg
	}
	switch {
	case m.quitStage == quitNow:
		return msg
	case m.quitStage == quitClearing:
		return nil
	case m.avatars.Holding():
		return imagesClearMsg{interrupt: interrupt}
	}
	return msg
}

// The stages of quitting while the terminal holds images.
const (
	// quitClearing waits the frame the deletes are written in.
	quitClearing = 1 + iota
	// quitNow lets the quit through.
	quitNow
)

// clearImages deletes the images from the terminal, and quits once the
// deletes have been written.
func (m *Model) clearImages(msg imagesClearMsg) tea.Cmd {
	m.quitStage = quitClearing
	return tea.Sequence(tea.Raw(m.avatars.Close()), m.after(quitFrame, imagesClearedMsg(msg)))
}

// imagesCleared quits, now that the deletes were written.
func (m *Model) imagesCleared(msg imagesClearedMsg) tea.Cmd {
	m.quitStage = quitNow
	if msg.interrupt {
		return tea.Interrupt
	}
	return tea.Quit
}

// hideAvatars has the avatars sent while the app's pane isn't on view
// sent again once it is: tmux with allow-passthrough on drops the
// passthrough of a pane that isn't, while all passes it.
func (m *Model) hideAvatars() {
	if m.graphics.Tmux && m.images.seen.Passthrough == "on" {
		m.avatars.Hide()
	}
}

// tick returns the command that sends msg after d.
func tick(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// ClearImages returns what deletes the images the app sent that are
// still in the terminal, as when the program ended without quitting, and
// sends no more. The caller writes it once the program has ended.
func (m *Model) ClearImages() string {
	return m.avatars.Close()
}
