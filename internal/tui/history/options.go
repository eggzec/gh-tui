package history

import (
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Option configures a Modal in [New].
type Option func(*options)

type options struct {
	cfg config.History
	// dates tell the dates of the rows and the commit pane.
	dates    ui.Dates
	prefetch prefetch
	// slots bound the reads ahead with those of the other pages.
	slots *ui.Slots
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
	// icons mark what failed to load, and verified signatures.
	icons ui.Icons
}

func defaultOptions() options {
	d := config.Default()
	return options{cfg: d.History, prefetch: newPrefetch(d.Prefetch), now: time.Now, loc: time.Local, icons: ui.NewIcons(d.UI.Icons)}
}

// prefetch is what the modal reads ahead: what the commits around the
// graph's cursor changed, and how far the branches around the branch
// pane's cursor are from the default branch.
type prefetch struct {
	commits, branches config.Resolved
}

func newPrefetch(p config.PrefetchLayers) prefetch {
	var r prefetch
	// The names are those of the settings, so these can't fail.
	r.commits, _ = p.Resolve("history", "commits")
	r.branches, _ = p.Resolve("history", "branches")
	return r
}

// WithConfig sets what the rows and the commit pane show, and how dates
// read. The default is that of config.Default.
func WithConfig(h config.History) Option {
	return func(o *options) { o.cfg = h }
}

// WithDates sets how dates read, as ui.date_format says. The default is
// as ages.
func WithDates(d ui.Dates) Option {
	return func(o *options) { o.dates = d }
}

// WithPrefetch sets how far the modal reads ahead, and how long a cursor
// rests before it does, as p, the prefetch settings, says for the
// history. The commit and the branch under the cursors are read once they
// rest whatever p says, since the panes show them. The default is that of
// config.Default.
func WithPrefetch(p config.PrefetchLayers) Option {
	return func(o *options) { o.prefetch = newPrefetch(p) }
}

// WithSlots bounds the reads ahead of the modal with those of every page
// and modal that shares s, so that together they keep to
// prefetch.parallel. Without it, each kind it reads has slots of its own.
func WithSlots(s *ui.Slots) Option {
	return func(o *options) { o.slots = s }
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

// WithIcons sets the icons whose glyphs mark what failed to load, and
// whether GitHub verified the signature of a commit.
// Without it, the icons are the config's default.
func WithIcons(ic ui.Icons) Option {
	return func(o *options) { o.icons = ic }
}

// onCommit opens the modal on the history of commit sha, with the commit
// pane focused on it, for [CommitOpener].
func onCommit(sha string) Option {
	return func(o *options) { o.commit = sha }
}
