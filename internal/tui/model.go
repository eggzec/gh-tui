// Package tui is the root of the program. It lays out a tab per section, the
// help line and toasts, and routes messages between them. The sections
// themselves live in their own packages and share the ui package.
package tui

import (
	"context"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/tabs"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Model is the root model of the program.
type Model struct {
	ctx      context.Context
	cfg      config.Config
	keys     KeyMap
	sections []ui.Section
	// started marks the sections whose Init has run.
	started []bool
	active  int

	tabs  tabs.Model
	toast toast.Model
	help  help.Model
	theme ui.Theme

	width, height int

	sync      func(ctx context.Context) (ui.SyncMsg, bool)
	setActive func(active bool)
	open      func(url string) error
	watchRepo func(repo core.RepoRef)
}

// Option configures a Model.
type Option func(*Model)

// WithSync sets the source of sync events. next blocks until the data behind
// a key may have changed, and reports false once there are no more events.
func WithSync(next func(ctx context.Context) (ui.SyncMsg, bool)) Option {
	return func(m *Model) { m.sync = next }
}

// WithActivity sets the function told whether the terminal has focus, so
// background polling can slow down while the user looks elsewhere.
func WithActivity(setActive func(active bool)) Option {
	return func(m *Model) { m.setActive = setActive }
}

// WithRepoWatcher sets the function told which repository is selected, so
// that the app can poll it for changes. It is called with every ui.RepoMsg,
// before the sections see it, and must not block.
func WithRepoWatcher(watch func(repo core.RepoRef)) Option {
	return func(m *Model) { m.watchRepo = watch }
}

// WithBrowser sets the function that opens a URL in the browser.
func WithBrowser(open func(url string) error) Option {
	return func(m *Model) { m.open = open }
}

// New returns the root model with a tab per section, in order. ctx bounds
// every request the app makes.
func New(ctx context.Context, cfg config.Config, sections []ui.Section, opts ...Option) *Model {
	titles := make([]string, len(sections))
	for i, s := range sections {
		titles[i] = s.Title()
	}
	keys := newKeyMap(cfg.Keys)
	m := &Model{
		ctx:      ctx,
		cfg:      cfg,
		keys:     keys,
		sections: sections,
		started:  make([]bool, len(sections)),
		tabs:     tabs.New(tabs.WithTabs(titles...), tabs.WithFocused(true), tabs.WithKeyMap(keys.Tabs)),
		toast:    toast.New(),
		help:     help.New(),
	}
	for _, opt := range opts {
		opt(m)
	}
	// Assume a dark terminal until it tells us otherwise.
	m.applyTheme(true)
	if len(sections) > 0 {
		sections[0].Focus()
	}
	return m
}

// Init asks for the terminal background, starts the first section and
// listens for sync events.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.start(m.active), m.listen())
}

// start initializes section i the first time it is shown.
func (m *Model) start(i int) tea.Cmd {
	if i >= len(m.sections) || m.started[i] {
		return nil
	}
	m.started[i] = true
	return m.sections[i].Init()
}

func (m *Model) listen() tea.Cmd {
	if m.sync == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := m.sync(m.ctx)
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *Model) applyTheme(dark bool) {
	p, err := m.cfg.Palette(dark)
	if err != nil {
		// A loaded config is validated, so this only guards a zero Config.
		p, _ = config.Default().Palette(dark)
	}
	m.theme = ui.NewTheme(p, dark)
	m.tabs.SetStyles(m.theme.Tabs())
	m.toast.SetStyles(m.theme.Toast())
	m.help.Styles = m.theme.Help()
	for _, s := range m.sections {
		s.SetTheme(m.theme)
	}
}

// layout gives each part its share of the screen.
func (m *Model) layout() {
	m.tabs.SetWidth(m.width)
	m.help.SetWidth(m.width)
	m.toast.SetSize(m.width, m.height)
	h := m.contentHeight()
	for _, s := range m.sections {
		s.SetSize(m.width, h)
	}
}

func (m *Model) contentHeight() int {
	return max(m.height-m.tabs.Height()-m.helpHeight(), 0)
}

// updateBadges copies the badges of the sections onto their tabs.
func (m *Model) updateBadges() {
	for i, s := range m.sections {
		b, ok := s.(ui.Badger)
		if !ok {
			continue
		}
		if badge := b.Badge(); badge != m.tabs.Badge(i) {
			m.tabs.SetBadge(i, badge)
		}
	}
}
