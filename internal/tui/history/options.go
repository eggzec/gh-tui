package history

import (
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Option configures a Modal in [New].
type Option func(*options)

type options struct {
	cfg     config.History
	offline *ui.Offline
	// now and loc read the clock and the time zone that dates are shown
	// in; tests fix them.
	now func() time.Time
	loc *time.Location
}

func defaultOptions() options {
	return options{cfg: config.Default().History, offline: new(ui.Offline), now: time.Now, loc: time.Local}
}

// WithConfig sets what the rows and the commit pane show, how dates read,
// and how far the modal reads ahead. The default is that of
// config.Default.
func WithConfig(h config.History) Option {
	return func(o *options) { o.cfg = h }
}

// WithOffline shares off with the sections, so that the user is told once
// for all of them that GitHub can't be reached. By default the modal has
// its own.
func WithOffline(off *ui.Offline) Option {
	return func(o *options) {
		if off != nil {
			o.offline = off
		}
	}
}
