package threads

import (
	"github.com/eggzec/gh-tui/internal/config"
)

// Configure takes the settings of c that the opener uses, which the set
// command changed: whether opening a thread marks it read, and the reads
// ahead, as WithMarkRead and WithPrefetch set them. The views that share
// it may each pass it the settings.
func (o *Opener) Configure(c config.Config) {
	if o == nil {
		return
	}
	o.markRead = c.Notifications.MarkReadOnOpen
	o.ahead.Configure(c.Prefetch)
}
