package threads

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Configure takes the settings of c that the opener uses, which the set
// command changed: the reads ahead, as WithPrefetch sets them. An opener
// without the pull requests and issues to read reads nothing ahead still.
// The views that share it may each pass it the settings.
func (o *Opener) Configure(c config.Config) {
	if o == nil {
		return
	}
	p := c.Details.Prefetch
	o.rows, o.delay = max(p.Rows, 0), p.HoverDelay
	switch {
	case !p.Enabled:
		// Resetting cancels the reads in flight.
		o.ahead.Reset(o.ctx)
		o.ahead, o.prefetch = nil, false
	case o.pulls == nil || o.issues == nil:
	case o.ahead == nil:
		o.prefetch = true
		o.ahead = ui.NewAhead("thread", o.read, o.current, o.rows, o.delay)
		o.ahead.Reset(o.ctx)
	default:
		o.ahead.Set(o.rows, o.delay)
	}
}
