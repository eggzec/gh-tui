package tui

import "github.com/eggzec/gh-tui/internal/target"

// RestartMsg asks the app to quit so that the program starts again as
// Target, on its host and as its login. The new start checks it again and
// exits on failure, so check it before sending this.
type RestartMsg struct{ Target target.Target }

// Restart returns the identity the app quit to start again as, and whether
// it quit for that.
func (m *Model) Restart() (target.Target, bool) {
	if m.restart == nil {
		return target.Target{}, false
	}
	return *m.restart, true
}
