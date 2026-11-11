// Package releaselist is the Releases pane of the repository screen: a list
// of the releases of the selected repository, newest first, with a flag for
// the latest, pre-release and draft ones and a count of their files. Enter
// opens the release in its modal.
package releaselist

import (
	"context"
	"strconv"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/releases"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// title names the pane.
const title = "Releases"

// ctxScreen is the context of the keys of the pane.
const ctxScreen = "releases"

// Service is what the pane reads.
type Service interface {
	Invalidate(repo core.RepoRef)
	List(ctx context.Context, q releases.ListQuery) (core.Page[core.Release], error)
}

// KeyMap holds the keys of the pane.
type KeyMap struct {
	Select  key.Binding
	Refresh key.Binding

	feed   feed.KeyMap
	search cmdline.KeyMap
}

func newKeyMap(keys config.Keymap) KeyMap {
	screen := ui.In(keys, ctxScreen)
	return KeyMap{
		Select:  screen.Binding("global.select", "open"),
		Refresh: screen.Binding("global.refresh", "refresh"),
		feed:    feed.NewKeyMap(screen),
		search:  ui.SearchPromptKeys(keys),
	}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding { return []key.Binding{k.Select, k.Refresh} }

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Select, k.Refresh}}
}

// Section lists the releases of a repository.
type Section struct {
	ctx   context.Context
	svc   Service
	keys  KeyMap
	now   func() time.Time
	icons ui.Icons
	dates ui.Dates
	theme ui.Theme
	looks [ui.NumLooks]cells

	feed    feed.Model[core.Release]
	repo    core.RepoRef
	started bool
	hasRepo bool
}

// live reports whether the pane has started and has a repository to read.
func (s *Section) live() bool { return s.started && s.hasRepo }

var (
	_ ui.Section = (*Section)(nil)
	_ ui.Counter = (*Section)(nil)
)

// New returns the section, which reads through svc.
func New(ctx context.Context, svc Service, keys config.Keymap) *Section {
	s := &Section{ctx: ctx, svc: svc, keys: newKeyMap(keys), now: time.Now, icons: ui.NewIcons(config.Default().UI.Icons)}
	voice := ui.NewVoice(keys, "")
	voice.Icons = &s.icons
	s.feed = feed.New(ui.FeedPages("list.releases", func(c string) releases.ListQuery { return releases.ListQuery{Cursor: c} }, s.list), s.render,
		feed.WithContext(ctx),
		feed.WithKey(func(r core.Release) string { return strconv.FormatInt(r.ID, 10) }),
		feed.WithKeyMap(s.keys.feed),
		feed.WithPromptKeys(s.keys.search),
		feed.WithErrorText(ui.ErrorText("load the releases", "", voice)),
		feed.WithMoreText("More releases are available."),
	)
	s.feed.SetEmptyText("No releases")
	return s
}

func (s *Section) list(ctx context.Context, q releases.ListQuery, again bool) (core.Page[core.Release], error) {
	q.Repo, q.Again = s.repo, again
	return s.svc.List(ctx, q)
}

// Title implements ui.Section.
func (s *Section) Title() string { return title }

// Init fetches the first page.
func (s *Section) Init() tea.Cmd {
	s.started = true
	if !s.hasRepo {
		return nil
	}
	return s.feed.Init()
}

// Update handles the pane's keys and the repository, and passes the rest to
// the list.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.RepoMsg:
		same := s.hasRepo && s.repo.Same(msg.Repo)
		s.repo, s.hasRepo = msg.Repo, true
		if !s.started || same {
			return nil
		}
		return s.feed.Reset()
	case ui.SyncMsg:
		if msg.Err == nil && s.live() && msg.Key == releases.SyncKey(s.repo) {
			return s.feed.Reload()
		}
		return nil
	case ui.OnlineMsg:
		return ui.RetryUnreached(&s.feed)
	case tea.KeyPressMsg:
		if s.feed.Focused() && !s.feed.Takes(msg) {
			switch {
			case keymap.Matches(msg, s.keys.Refresh):
				s.svc.Invalidate(s.repo)
				return s.feed.Reload()
			case keymap.Matches(msg, s.keys.Select):
				return s.open()
			}
		}
	}
	var cmd tea.Cmd
	s.feed, cmd = s.feed.Update(msg)
	return cmd
}

func (s *Section) open() tea.Cmd {
	r, ok := s.feed.Selected()
	if !ok {
		return nil
	}
	repo := s.repo
	return func() tea.Msg { return ui.OpenReleaseMsg{Repo: repo, ID: r.ID, URL: r.URL} }
}

// SetSize implements ui.Section.
func (s *Section) SetSize(width, height int) { s.feed.SetSize(width, height) }

// SetTheme restyles the pane.
func (s *Section) SetTheme(t ui.Theme) {
	s.theme = t
	for i := range s.looks {
		s.looks[i] = newCells(t, s.icons, i)
	}
	s.feed.SetStyles(t.Feed(s.icons))
}

// Focus implements ui.Section.
func (s *Section) Focus() { s.feed.Focus() }

// Blur implements ui.Section.
func (s *Section) Blur() { s.feed.Blur() }

// KeyLayers implements ui.Keyed.
func (s *Section) KeyLayers() []keyhelp.Layer {
	list, prompting := ui.FeedLayer(s.feed)
	if prompting {
		return []keyhelp.Layer{list}
	}
	own := keyhelp.FromHelp("", s.keys, false)
	return []keyhelp.Layer{ui.MergeLayers(ctxScreen, own, list)}
}

// View draws the list.
func (s *Section) View() string { return s.feed.View() }

// CountForms is the forms of the count of the releases for the pane's frame,
// longest first (see ui.CountForms). There is none while the list loads. The
// list has no total: it is unknown until the last page is in.
func (s *Section) CountForms() []string {
	f := &s.feed
	if f.Len() == 0 && (!f.Done() || f.Err() != nil) {
		return nil
	}
	return ui.CountForms(ui.CountSpec{Loaded: f.Len(), More: !f.Done()})
}
