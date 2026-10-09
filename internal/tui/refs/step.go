// Package refs is the References step of the modal of a pull request or an
// issue: the issues and pull requests linked to it, in groups that fold.
// "Closes" (or "Closed by") holds what GitHub says the pull request closes,
// or what closed the issue; "Written here" what its body, comments and
// reviews link; and "Mentioned in" what other items mention it, which is
// read a page at a time, only once it opens. A row opens its item in
// place of the modal, and the back key returns to the step as it was left.
package refs

import (
	"context"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	refssvc "github.com/eggzec/gh-tui/internal/service/refs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/tree"
)

// Item is what the step asks the modal about the item on view, which
// changes as the modal reads it.
type Item struct {
	// Title is shown over the links, and Updated is when GitHub says the
	// item last changed, or zero if that isn't known yet: links read
	// before it are read again.
	Title   string
	Updated time.Time
}

// Option configures a Step in [New].
type Option func(*options)

type options struct {
	icons    ui.Icons
	ret      ui.Modal
	host     string
	pageSize int
	item     func() Item
	voice    *ui.Voice
	now      func() time.Time
}

// WithReturn sets the modal the step is shown in, which an item picked in
// the step replaces, and which the back key returns to.
func WithReturn(m ui.Modal) Option {
	return func(o *options) { o.ret = m }
}

// WithIcons sets the glyphs of the states of the items. Without it, the
// icons are the config's default.
func WithIcons(ic ui.Icons) Option {
	return func(o *options) { o.icons = ic }
}

// WithHost sets the host of the session's web pages, which the key that
// opens an item that can't be read goes to. The default is github.com.
func WithHost(host string) Option {
	return func(o *options) { o.host = host }
}

// WithPageSize sets how many mentions a page holds, which words the row
// that reads the next. It is the size the service reads them in.
func WithPageSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.pageSize = n
		}
	}
}

// WithItem sets how the step learns the title of the item on view and when
// it was last updated. Without it, the step shows no title.
func WithItem(f func() Item) Option {
	return func(o *options) { o.item = f }
}

// WithVoice sets how the step words what went wrong, with the keys a hint
// names and the log it points to. By default the hints name the configured
// keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(o *options) { o.voice = &v }
}

// WithClock sets the clock that the times of the notes count to. The
// default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

var lastID atomic.Int64

// Step is the References step. Create one with [New], and Init it once it
// is shown.
type Step struct {
	id int64
	// ctx bounds the step's reads, and cancel ends them when it closes.
	ctx    context.Context
	cancel context.CancelFunc
	svc    Service
	self   core.Target
	// pull says that the item on view is a pull request.
	pull bool
	keys KeyMap
	opts options

	tree tree.Model
	// started is set once the tree was started, which waits for the first
	// links, and folds while the groups still have to be opened or closed as
	// they start.
	started, folds bool
	// snap is what the tree reads of the links. pages is how many pages of
	// mentions the tree asks for, again that the next read of them goes
	// past the cache, and mread what the last read of them found.
	snap  atomic.Pointer[snapshot]
	pages atomic.Int32
	again atomic.Bool
	mread atomic.Pointer[mentionsRead]

	// asked is set once the filter in force has had the first page of the
	// mentions asked for, so that a failing read isn't tried on every key.
	// landFrom is how many mentions were listed when the row of more was
	// picked, until the cursor has moved to the first of those that came,
	// or -1.
	asked    bool
	landFrom int

	refs core.References
	// loaded is set once links arrived, loading while they are read, and
	// err once a read failed with none to show. seq numbers the reads, so
	// that the answers of earlier ones are dropped.
	loaded, loading bool
	err             error
	seq             int

	// prompt is the prompt of the filter, open while it takes keys, and
	// filter the text the rows are filtered by.
	prompt cmdline.Model
	filter string

	spin     spinner.Model
	spinning bool
	voice    ui.Voice

	width, height int
	theme         ui.Theme
	st            styles
	errs          ui.ErrorStyles
}

// New returns the References step of the item self, a pull request if pull
// is set, with the configured keys. ctx bounds its reads until it closes.
func New(ctx context.Context, svc Service, self core.Target, pull bool, keys config.Keymap, opts ...Option) *Step {
	o := options{icons: ui.NewIcons(config.Default().UI.Icons), host: core.DefaultHost, pageSize: config.Default().PageSize.References, now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}
	rctx, cancel := context.WithCancel(ctx)
	s := &Step{
		id:     lastID.Add(1),
		ctx:    rctx,
		cancel: cancel,
		svc:    svc,
		self:   self,
		pull:   pull,
		keys:   newKeyMap(keys),
		opts:   o,
		spin:   spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	s.pages.Store(1)
	s.landFrom = -1
	v := ui.NewVoice(keys, "")
	if o.voice != nil {
		v = *o.voice
	}
	v.Retry, v.Icons = s.keys.Refresh, &s.opts.icons
	if v.Now == nil {
		v.Now = o.now
	}
	s.voice = v
	s.prompt = cmdline.New(0, cmdline.WithPrompt(promptFilter), cmdline.WithKeyMap(cmdline.KeyMap{CancelEmpty: s.keys.prompt.cancelEmpty}))
	s.tree = tree.New(s.children,
		tree.WithContext(rctx),
		tree.WithKeyMap(s.keys.Tree),
		tree.WithFocused(true),
		// The titles are what the rows are for: the state at the right gives
		// way to them.
		tree.WithMinName(minName),
		tree.WithEmptyText("No link matches the filter."),
		tree.WithErrorText(ui.ErrorText("load the mentions", self.String(), s.voice)),
	)
	// Assume a dark terminal until the parent sets the theme.
	p, _ := config.Default().Palette(true)
	s.SetTheme(ui.NewTheme(p, true))
	if r, ok := svc.CachedReferences(s.query(false)); ok {
		s.setRefs(r)
	}
	return s
}

// minName is the fewest cells of a title that a row keeps to show the state
// beside it.
const minName = 24

// promptFilter is the prompt of the filter, which also says what the line
// typed after it is for.
const promptFilter = "&"

// ID returns the instance ID that scopes the step's messages.
func (s *Step) ID() int64 { return s.id }

// CloseMsg asks the parent to close the modal the step with ID is in, which
// the dismiss key sends once there is no filter left to clear.
type CloseMsg struct {
	ID int64
}

// Init starts the tree if links are cached, and reads them, which a fresh
// cached copy answers without a request.
func (s *Step) Init() tea.Cmd {
	return tea.Batch(s.start(), s.read(false))
}

// Close ends the step's reads.
func (s *Step) Close() {
	s.cancel()
}

// TakesKeys reports whether the step types the keys into the prompt of its
// filter, so that the parent leaves it every one.
func (s *Step) TakesKeys() bool {
	return s.prompt.Focused()
}

// Filter returns the text the rows are filtered by, or "".
func (s *Step) Filter() string {
	return s.filter
}

// query returns the query of the links.
func (s *Step) query(again bool) refssvc.Query {
	q := refssvc.Query{Repo: s.self.Repo, Number: s.self.Number, Pull: s.pull, Again: again}
	if s.opts.item != nil {
		q.Updated = s.opts.item().Updated
	}
	return q
}

// startSpinner starts the spinner while something loads, unless it runs.
func (s *Step) startSpinner() tea.Cmd {
	if s.spinning {
		return nil
	}
	s.spinning = true
	return s.spin.Tick
}
