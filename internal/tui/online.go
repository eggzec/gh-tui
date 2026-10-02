package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// onlineGap is the shortest time between two wakes after GitHub answers
// again. A connection that flaps, such as on a roaming Wi-Fi or a VPN
// that reconnects, goes on and off line every second or two, and each
// wake reads again what failed, which mostly fails again while it
// flaps. Waking at most once a gap caps that at a read per failed pane
// and poll every 10s, while the first answer after an outage still wakes
// them at once.
const onlineGap = 10 * time.Second

// onlineTickMsg ends the wait of a wake that came within onlineGap of the
// one before.
type onlineTickMsg struct{}

// cameOnline wakes what failed while GitHub couldn't be reached, now that
// it answers again: the polls and passes that backed off, through
// m.online, and the sections, with a ui.OnlineMsg. A wake within
// onlineGap of the last waits until the gap has passed, and happens then
// only if GitHub still answers.
func (m *Model) cameOnline() tea.Cmd {
	now := time.Now()
	wait := m.wokeAt.Add(onlineGap).Sub(now)
	if wait <= 0 {
		return m.wake(now)
	}
	if m.waking {
		return nil
	}
	m.waking = true
	return tea.Tick(wait, func(time.Time) tea.Msg { return onlineTickMsg{} })
}

// onlineTick wakes what failed, once the wait for onlineGap is over, if
// GitHub still answers. If it doesn't, the next answer wakes them.
func (m *Model) onlineTick() tea.Cmd {
	m.waking = false
	if !m.offSince.IsZero() {
		return nil
	}
	return m.wake(time.Now())
}

func (m *Model) wake(now time.Time) tea.Cmd {
	m.wokeAt = now
	if m.online != nil {
		m.online()
	}
	cmd := m.broadcast(ui.OnlineMsg{Limited: limitedUntil(m.rate).After(now)})
	// The avatars that failed ask again as they are drawn again.
	if m.avatars.Online() {
		cmd = tea.Batch(cmd, m.avatarsChanged())
	}
	return cmd
}
