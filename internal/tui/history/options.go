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
	// voice words the errors of the graph; New makes one of its keys if
	// it is nil.
	voice *ui.Voice
	// host is the web host of the user's GitHub, for the links it opens.
	host string
	// now and loc read the clock and the time zone that dates are shown
	// in; tests fix them.
	now func() time.Time
	loc *time.Location
	// commit, if set, is the commit the modal opens on, in place of a
	// branch.
	commit string
	// editor is the editor the pager opens a patch in, if set.
	editor string
	// icons mark what failed to load.
	icons ui.Icons
}

func defaultOptions() options {
	return options{cfg: config.Default().History, offline: new(ui.Offline), now: time.Now, loc: time.Local, icons: ui.NewIcons(config.Default().UI.Icons)}
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

// WithVoice sets how the modal words what went wrong, with the keys a
// hint names and the log it points to. By default the hints name the
// configured keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(o *options) { o.voice = &v }
}

// WithHost sets the web host of the user's GitHub, with its port if it has
// one, whose pages the modal opens. It defaults to github.com.
func WithHost(host string) Option {
	return func(o *options) { o.host = host }
}

// WithEditor sets the command of the editor that the pager opens a patch
// in, before $VISUAL and $EDITOR, as pager.WithEditor takes it.
func WithEditor(cmd string) Option {
	return func(o *options) { o.editor = cmd }
}

// WithIcons sets the icons whose error glyph marks what failed to load.
// Without it, the icons are the config's default.
func WithIcons(ic ui.Icons) Option {
	return func(o *options) { o.icons = ic }
}

// onCommit opens the modal on the history of commit sha, with the commit
// pane focused on it, for [CommitOpener].
func onCommit(sha string) Option {
	return func(o *options) { o.commit = sha }
}
