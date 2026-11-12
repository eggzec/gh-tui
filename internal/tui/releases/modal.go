// Package releases is the modal of a release: its author, tag and target over
// its notes, rendered as markdown, and the files uploaded with it, in two
// panes. The app opens it from a notification of the release.
package releases

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// Service is what the modal needs of the releases service.
type Service interface {
	CachedGet(repo core.RepoRef, id int64) (core.Release, bool)
	Get(ctx context.Context, repo core.RepoRef, id int64) (core.Release, error)
	// AssetURL resolves the address a file of the release is read from.
	AssetURL(ctx context.Context, repo core.RepoRef, id int64) (string, error)
	// CachedExtra and Extra read what the header shows beyond the release:
	// its commit, how far the default branch moved and who is mentioned.
	CachedExtra(repo core.RepoRef, id int64) (core.ReleaseExtra, bool)
	Extra(ctx context.Context, repo core.RepoRef, id int64) (core.ReleaseExtra, error)
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
	// links confirms the references of the release's notes.
	links *ui.Autolinks
	// downloader saves the files; without it the download keys do nothing.
	downloader Downloader
	// pics draws the avatars of the header; nil draws handles instead.
	pics *ui.Images
}

// WithAvatars sets the pool that the avatars of the header are drawn from.
// Without it, the header names people by their handles.
func WithAvatars(pics *ui.Images) Option {
	return func(o *options) { o.pics = pics }
}

// WithAutolinks links the references (#N, @login, hashes) of the release's
// notes that links confirmed. Without it, they stay text.
func WithAutolinks(links *ui.Autolinks) Option {
	return func(o *options) { o.links = links }
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

// extraMsg carries what the header shows beyond the release.
type extraMsg struct {
	id    int64
	extra core.ReleaseExtra
	err   error
}

// Modal shows a release. Create one with [New].
type Modal struct {
	// extra is what the header shows beyond the release, once extraDone is
	// set, and extraAsked that it was asked for.
	extra      core.ReleaseExtra
	extraDone  bool
	extraAsked bool
	// pics draws the avatars; it may be nil.
	pics *ui.Images

	id   int64
	svc  Service
	keys keyMap
	now  func() time.Time
	// drew is the generation of the autolinks the thread was drawn with.
	drew uint64
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
	links  *ui.Autolinks
	loaded bool
	err    error

	nav        panes.Nav
	zoom       panes.Zoom
	frames     panes.Styles
	dividers   *ui.Dividers
	treeKeys   treeview.KeyMap
	notes      treeview.Model
	files      treeview.Model
	md         *markdown.Renderer
	notesMade  bool
	notesBuilt notesKey
	look       int
	// ctx bounds the reads of the modal and is cancelled when it closes.
	ctx    context.Context
	cancel context.CancelFunc
	closed bool

	// voice words why the release failed to load.
	voice ui.Voice
	// icons draw the modal and mark what failed to load.
	icons ui.Icons

	// dl saves the files, and pick is the folder picker that shows while a
	// download waits for its folder.
	dl          Downloader
	pick        *picking
	pickKeys    pickKeys
	confirmKeys confirm.KeyMap

	width, height int
	theme         ui.Theme
	errs          ui.ErrorStyles
}

var _ ui.Modal = (*Modal)(nil)

// Opener returns what the app opens a release with, for tui.WithRelease: a
// new modal each time, which reads the release from svc and takes its keys
// from the configured keys.
func Opener(svc Service, keys config.Keymap, opts ...Option) func(ctx context.Context, repo core.RepoRef, id int64, url string) (ui.Modal, tea.Cmd) {
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
func New(ctx context.Context, svc Service, repo core.RepoRef, id int64, url string, keys config.Keymap, opts ...Option) *Modal {
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
		links:  o.links,
		pics:   o.pics,

		dl:          o.downloader,
		pickKeys:    newPickKeys(keys),
		confirmKeys: confirm.NewKeyMap(ui.Lookup(keys, "confirm")),
	}
	// The voice reads the icons of the modal itself, not of the options,
	// which go out of use.
	m.voice.Icons = &m.icons
	m.dividers = &ui.Dividers{Step: 4}
	m.nav.SetFocus(paneNotes)
	km := treeview.NewKeyMap(ui.Lookup(keys, ctxModal))
	// A row has nothing to open here, so enter is off in the help too.
	km.Open.SetEnabled(false)
	m.treeKeys = km
	m.notes = treeview.New(treeview.Source{}, treeview.WithKeyMap(km))
	m.files = treeview.New(treeview.Source{}, treeview.WithKeyMap(km))
	r, cached := svc.CachedGet(repo, id)
	slog.InfoContext(ctx, "open", "span", "tui", "kind", "release", "repo", repo.String(), "id", id, "cached", cached)
	if cached {
		m.rel, m.loaded = r, true
	}
	if x, ok := svc.CachedExtra(repo, id); ok {
		m.extra, m.extraDone = x, true
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
		return tea.Batch(m.show(), m.get(), m.getExtra())
	}
	return m.get()
}

// getExtra reads what the header shows beyond the release, once.
func (m *Modal) getExtra() tea.Cmd {
	if m.extraAsked {
		return nil
	}
	m.extraAsked = true
	svc, ctx, repo, rid, id := m.svc, m.ctx, m.repo, m.rid, m.id
	return func() tea.Msg {
		start := time.Now()
		x, err := svc.Extra(ctx, repo, rid)
		obs.End(ctx, start, err, "span", "tui", "repo", repo.String(), "release", rid, "part", "extra")
		return extraMsg{id: id, extra: x, err: err}
	}
}

// resize sizes the trees to the panes.
func (m *Modal) resize() {
	for _, b := range m.paneLayout().Boxes {
		in := b.Rect.Inner()
		switch b.Pane {
		case paneNotes:
			m.notes.SetSize(in.W, in.H)
		case paneFiles:
			m.files.SetSize(in.W, in.H)
		}
	}
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
	m.rel, m.loaded, m.err = msg.rel, true, nil
	return tea.Batch(m.show(), m.getExtra())
}

// show builds the rows from the release and has the autolinks confirmed.
func (m *Modal) show() tea.Cmd {
	m.notesMade = false
	m.buildFiles()
	m.resize()
	return m.links.Verify(m.ctx, m.repo.String(), m.rel.Body)
}

// Title implements ui.Modal.
func (m *Modal) Title() string {
	name := "Release"
	if m.loaded {
		name = releaseName(m.rel)
	}
	return ui.Title(ui.TitleSpec{Name: name, Chips: []string{m.repo.String()}, Sep: m.icons.Separator})
}

// TrimTitle implements ui.Trimmed: the tag keeps its place in the top edge of
// the frame, after a glyph for the state, and the name of the release, when it
// differs from the tag, is cut first.
func (m *Modal) TrimTitle() ui.TrimTitle {
	t := ui.TrimTitle{Name: "Release"}
	if !m.loaded {
		return t
	}
	r := m.rel
	t.Name = ui.OneLine(r.Tag)
	if name := strings.TrimSpace(ui.OneLine(r.Name)); name != "" && name != t.Name {
		t.Detail = name
	}
	if t.Name == "" {
		t.Name, t.Detail = releaseName(r), ""
	}
	state := ui.ReleasePublished
	switch {
	case r.Draft:
		state = ui.ReleaseDraft
	case r.Prerelease:
		state = ui.ReleasePre
	}
	t.Right = releaseDate(r)
	t.Lead = m.theme.State(state).Render(m.icons.State(state))
	return t
}

// Link implements ui.Linked: the page of the release, or of the releases
// until it is read.
func (m *Modal) Link() string {
	if m.loaded && m.rel.URL != "" {
		return m.rel.URL
	}
	return m.url
}

// Commands implements ui.Commanded: the release can be copied.
func (m *Modal) Commands() []string { return []string{ui.CommandCopy} }

// Selected implements ui.Selector, for the copy command: the release the
// modal shows. A release has no number and no commit of its own: its ref
// is its tag, once it is read.
func (m *Modal) Selected() (ui.Selection, bool) {
	sel := ui.Selection{What: "release", URL: m.Link(), Repo: m.repo, Owner: m.repo.Owner}
	if m.loaded {
		sel.Tag = m.rel.Tag
	}
	return sel, true
}

// SetSize implements ui.Modal.
func (m *Modal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.resize()
	if m.pick != nil {
		m.pick.fp.SetHeight(m.pickHeight())
	}
}

// SetTheme implements ui.Modal.
func (m *Modal) SetTheme(t ui.Theme) {
	m.voice.Icons = &m.icons
	m.theme = t
	m.errs = t.Errors(m.icons)
	m.frames = panes.DefaultStyles(t.Dark)
	m.frames.Ellipsis, m.frames.Separator = m.icons.Ellipsis, m.icons.Separator
	m.md = ui.NewMarkdown(t, m.icons)
	m.md.SetStyle(t.Thread(m.icons).Markdown)
	m.md.SetGlyphs(m.icons.Markdown)
	m.md.SetAutolink(m.links.Func(m.repo.String()))
	m.look++
	m.notes.SetStyles(t.Tree(m.icons))
	m.files.SetStyles(t.Tree(m.icons))
	if m.loaded {
		m.buildFiles()
	}
}

// Update implements ui.Modal. After the modal closed, it ignores what
// arrives late.
func (m *Modal) Update(msg tea.Msg) tea.Cmd {
	if m.closed {
		return nil
	}
	if _, isKey := msg.(tea.KeyPressMsg); !isKey && m.pick != nil {
		// The picker reads its folder in a command, and the answer comes
		// back as a message of its own. The modal still takes its own
		// messages, which the picker ignores.
		return tea.Batch(m.updatePick(msg), m.update(msg))
	}
	return m.update(msg)
}

// update handles one message of the modal.
func (m *Modal) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.SettingsMsg:
		msg.ReloadIcons(&m.icons, &m.voice)
		return nil
	case tea.KeyPressMsg:
		return m.press(msg)
	case releaseMsg:
		if msg.id != m.id {
			return nil
		}
		return m.receive(msg)
	case extraMsg:
		if msg.id != m.id {
			return nil
		}
		if msg.err == nil {
			m.extra, m.extraDone = msg.extra, true
			m.resize()
		} else if ui.Unreached(msg.err) {
			// Read again when GitHub answers; see online.
			m.extraAsked = false
		}
		return nil
	case ui.ImagesMsg:
		// The header draws its avatars on every View; nothing is kept.
		return nil
	case ui.OnlineMsg:
		return m.online()
	case ui.SyncMsg:
		if m.links.Synced(msg, &m.drew) {
			m.md.SetAutolink(m.links.Func(m.repo.String()))
			m.look++
		}
	}
	return nil
}

// KeyLayers implements ui.Keyed: the modal's own keys, with retry while a
// read failed, and then the thread's, which take no keys while nothing
// shows.
func (m *Modal) KeyLayers() []keyhelp.Layer {
	if m.pick != nil {
		return []keyhelp.Layer{ui.MergeLayers(ctxModal, keyhelp.FromHelp("", m.pickKeys, false))}
	}
	k := m.keys
	k.Select.SetEnabled(k.Select.Enabled() && m.nav.Focus() == paneFiles)
	k.Refresh.SetEnabled(k.Refresh.Enabled() && m.failed())
	if m.failed() {
		k.offFailed()
	}
	own := keyhelp.FromHelp("", k, false)
	if m.failed() {
		return []keyhelp.Layer{ui.MergeLayers(ctxModal, own)}
	}
	return []keyhelp.Layer{ui.MergeLayers(ctxModal, own, keyhelp.FromHelp("", m.treeKeys, false))}
}

// online reads again, now that GitHub answers again, the release and its
// files if they failed for want of an answer from it.
func (m *Modal) online() tea.Cmd {
	if m.failed() && ui.Unreached(m.err) {
		m.err = nil
		return m.get()
	}
	if m.loaded && !m.extraDone {
		return m.getExtra()
	}
	return nil
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
