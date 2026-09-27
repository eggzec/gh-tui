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

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
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
	loc  *time.Location

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

	width, height int
	theme         ui.Theme
	st            styles
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
	o := options{now: time.Now, loc: time.Local}
	for _, opt := range opts {
		opt(&o)
	}
	ctx, cancel := context.WithCancel(obs.WithTrace(ctx, "open.release"))
	m := &Modal{
		id:     lastID.Add(1),
		svc:    svc,
		keys:   newKeyMap(keys),
		now:    o.now,
		loc:    o.loc,
		repo:   repo,
		rid:    id,
		url:    url,
		ctx:    ctx,
		cancel: cancel,
	}
	fetch := func(ctx context.Context, _ string) ([]core.ReleaseAsset, string, error) {
		r, err := svc.Get(ctx, repo, id)
		return r.Assets, "", err
	}
	m.thread = thread.New(fetch, m.renderAsset,
		thread.WithContext(ctx),
		thread.WithKeyMap(m.keys.thread),
		thread.WithFocused(true),
		thread.WithEmptyText("No files were uploaded with it."),
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
	return name + " · " + m.repo.String()
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
	m.st = newStyles(t)
	m.thread.SetStyles(t.Thread())
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
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}

// Help implements ui.Modal.
func (m *Modal) Help() help.KeyMap {
	k := m.keys
	retry := k.Refresh
	retry.SetEnabled(retry.Enabled() && m.failed())
	return m.keys.help(retry, m.thread.OnDiagram())
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
