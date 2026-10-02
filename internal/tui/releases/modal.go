// Package releases is the modal of a release: its tag, name, author and
// date over its notes, rendered as markdown, and the files uploaded with
// it. The app opens it from a notification of the release.
package releases

import (
	"context"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
)

// Service is what the modal needs of the releases service.
type Service interface {
	CachedGet(repo core.RepoRef, id int64) (core.Release, bool)
	Get(ctx context.Context, repo core.RepoRef, id int64) (core.Release, error)
}

// Option configures the modal.
type Option func(*options)

type options struct {
	now func() time.Time
	loc *time.Location
	// dates tell when the release was published.
	dates ui.Dates
	// voice words the errors of the files; New makes one of its keys if
	// it is nil.
	voice *ui.Voice
	// icons draw the modal and mark what failed to load.
	icons ui.Icons
}

// WithNow sets the clock that ages are measured against. The default is
// time.Now.
func WithNow(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

// WithLocation sets the time zone that dates are shown in. The default is
// time.Local.
func WithLocation(loc *time.Location) Option {
	return func(o *options) { o.loc = loc }
}

// WithDates sets how dates read, as ui.date_format says. The default is
// as ages.
func WithDates(d ui.Dates) Option {
	return func(o *options) { o.dates = d }
}

// WithVoice sets how the modal words what went wrong, with the keys a
// hint names and the log it points to. By default the hints name the
// configured keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(o *options) { o.voice = &v }
}

// WithIcons sets the icons that draw the modal and mark what failed to
// load. Without it, the icons are the config's default.
func WithIcons(ic ui.Icons) Option {
	return func(o *options) { o.icons = ic }
}

// releaseMsg carries the release that a modal asked for.
type releaseMsg struct {
	id  int64
	rel core.Release
	err error
}

// Modal shows a release. Create one with [New].
type Modal struct {
	id   int64
	svc  Service
	keys keyMap
	now  func() time.Time
	// dates tell when the release was published, in the zone of the
	// modal.
	dates ui.Dates

	repo core.RepoRef
	rid  int64
	// url is the page the open key shows until the release is loaded,
	// such as the releases of the repository.
	url string
	// rel is the release once loaded is set, and err why the last read
	// failed.
	rel    core.Release
	loaded bool
	err    error

	thread thread.Model[core.ReleaseAsset]
	// ctx bounds the reads of the modal and is cancelled when it closes.
	ctx    context.Context
	cancel context.CancelFunc
	closed bool

	// voice words why the release failed to load.
	voice ui.Voice
	// icons draw the modal and mark what failed to load.
	icons ui.Icons

	width, height int
	theme         ui.Theme
	st            styles
	errs          ui.ErrorStyles
}

var _ ui.Modal = (*Modal)(nil)

// Opener returns what the app opens a release with, for tui.WithRelease: a
// new modal each time, which reads the release from svc and takes its keys
// from the configured keys.
func Opener(svc Service, keys map[string][]string, opts ...Option) func(ctx context.Context, repo core.RepoRef, id int64, url string) (ui.Modal, tea.Cmd) {
	return func(ctx context.Context, repo core.RepoRef, id int64, url string) (ui.Modal, tea.Cmd) {
		m := New(ctx, svc, repo, id, url, keys, opts...)
		load := m.Init()
		return m, load
	}
}

var lastID atomic.Int64

// New returns the modal of release id of repo. url is the page the open
// key shows until the release is loaded. ctx bounds its reads until it
// closes. Call Init once it is open. A cached release shows at once.
func New(ctx context.Context, svc Service, repo core.RepoRef, id int64, url string, keys map[string][]string, opts ...Option) *Modal {
	o := options{now: time.Now, loc: time.Local, icons: ui.NewIcons(config.Default().UI.Icons)}
	for _, opt := range opts {
		opt(&o)
	}
	if o.voice == nil {
		v := ui.NewVoice(keys, "")
		o.voice = &v
	}
	ctx, cancel := context.WithCancel(obs.WithTrace(ctx, "open.release"))
	m := &Modal{
		id:     lastID.Add(1),
		svc:    svc,
		keys:   newKeyMap(keys),
		now:    o.now,
		dates:  o.dates.In(o.loc),
		repo:   repo,
		rid:    id,
		url:    url,
		ctx:    ctx,
		cancel: cancel,
		voice:  *o.voice,
		icons:  o.icons,
	}
	fetch := func(ctx context.Context, _ string) ([]core.ReleaseAsset, string, error) {
		r, err := svc.Get(ctx, repo, id)
		return r.Assets, "", err
	}
	// The files come with the release, which the thread has no key to
	// read again.
	v := *o.voice
	v.Retry = m.keys.thread.Retry
	m.thread = thread.New(fetch, m.renderAsset,
		thread.WithContext(ctx),
		thread.WithKeyMap(m.keys.thread),
		thread.WithFocused(true),
		thread.WithEmptyText("No assets."),
		thread.WithErrorText(ui.ErrorText("load the release", repo.String(), v)),
	)
	m.thread.SetCutHint(ui.OpenHint(m.keys.Open))
	r, cached := svc.CachedGet(repo, id)
	slog.InfoContext(ctx, "open", "span", "tui", "kind", "release", "repo", repo.String(), "id", id, "cached", cached)
	if cached {
		m.rel, m.loaded = r, true
		// The files show without a read.
		m.thread.SetFirst(r.Assets, "")
	}
	// Assume a dark terminal until the app sets the theme.
	p, _ := config.Default().Palette(true)
	m.SetTheme(ui.NewTheme(p, true))
	return m
}

// Init shows the cached release, and reads it again behind it, or reads
// it for the first time.
func (m *Modal) Init() tea.Cmd {
	if m.loaded {
		return tea.Batch(m.show(), m.get())
	}
	return tea.Batch(m.thread.Init(), m.get())
}

// get reads the release. A fresh cached release costs no request.
func (m *Modal) get() tea.Cmd {
	svc, ctx, repo, rid, id := m.svc, m.ctx, m.repo, m.rid, m.id
	return func() tea.Msg {
		start := time.Now()
		r, err := svc.Get(ctx, repo, rid)
		obs.End(ctx, start, err, "span", "tui", "repo", repo.String(), "release", rid)
		return releaseMsg{id: id, rel: r, err: err}
	}
}

// receive shows the release read, or why it couldn't be read.
func (m *Modal) receive(msg releaseMsg) tea.Cmd {
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return nil
		}
		m.err = msg.err
		return nil
	}
	changed := m.loaded && !slices.Equal(m.rel.Assets, msg.rel.Assets)
	first := !m.loaded
	m.rel, m.loaded, m.err = msg.rel, true, nil
	if first {
		m.thread.SetFirst(m.rel.Assets, "")
	}
	cmd := m.show()
	if changed {
		cmd = tea.Batch(cmd, m.thread.Reload())
	}
	return cmd
}

// show sets the document of the thread from the release.
func (m *Modal) show() tea.Cmd {
	return m.thread.SetDocument(m.header(m.width), m.rel.Body)
}

// Title implements ui.Modal.
func (m *Modal) Title() string {
	name := "Release"
	if m.loaded {
		name = releaseName(m.rel)
	}
	return name + m.icons.Separator + m.repo.String()
}

// Link implements ui.Linked: the page of the release, or of the releases
// until it is read.
func (m *Modal) Link() string {
	if m.loaded && m.rel.URL != "" {
		return m.rel.URL
	}
	return m.url
}

// SetSize implements ui.Modal.
func (m *Modal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.thread.SetSize(m.width, m.height)
	if m.loaded {
		// The header wraps to the width.
		_ = m.show()
	}
}

// SetTheme implements ui.Modal.
func (m *Modal) SetTheme(t ui.Theme) {
	m.theme = t
	m.st = newStyles(t, m.icons)
	m.errs = t.Errors(m.icons)
	m.thread.SetStyles(t.Thread(m.icons))
	if m.loaded {
		_ = m.show()
	}
}

// Update implements ui.Modal. After the modal closed, it ignores what
// arrives late.
func (m *Modal) Update(msg tea.Msg) tea.Cmd {
	if m.closed {
		return nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.press(msg)
	case releaseMsg:
		if msg.id != m.id {
			return nil
		}
		return m.receive(msg)
	case ui.OnlineMsg:
		return m.online()
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}

// KeyLayers implements ui.Keyed: the modal's own keys, with retry while a
// read failed, and then the thread's, which take no keys while nothing
// shows.
func (m *Modal) KeyLayers() []keyhelp.Layer {
	k := m.keys
	k.Refresh.SetEnabled(k.Refresh.Enabled() && m.failed())
	doc := keyhelp.FromHelp("thread", m.thread, false)
	if m.failed() {
		doc = ui.Off(doc)
	}
	return []keyhelp.Layer{keyhelp.FromHelp("release", k, false), doc}
}

// online reads again, now that GitHub answers again, the release and its
// files if they failed for want of an answer from it.
func (m *Modal) online() tea.Cmd {
	var get tea.Cmd
	if m.failed() && ui.Unreached(m.err) {
		m.err = nil
		get = m.get()
	}
	return tea.Batch(get, ui.RetryUnreached(&m.thread))
}

// failed reports whether the release couldn't be read and nothing shows.
func (m *Modal) failed() bool {
	return !m.loaded && m.err != nil
}

// close ends the modal's reads and asks the app to close it.
func (m *Modal) close() tea.Cmd {
	m.closed = true
	m.cancel()
	return ui.CloseModal(m)
}
