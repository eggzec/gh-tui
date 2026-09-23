// Package repos is the Repositories section: the viewer's repositories,
// which can be starred, opened in the browser, or chosen as the repository
// the other sections show.
package repos

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// Title is the title of the section's tab.
const Title = "Repositories"

// pullsTitle is the section that choosing a repository shows.
const pullsTitle = "Pull requests"

// Service is what the section needs from the repositories service.
type Service interface {
	List(ctx context.Context, q reposvc.ListQuery) (core.Page[core.Repo], error)
	Get(ctx context.Context, ref core.RepoRef) (core.Repo, error)
	Star(ref core.RepoRef) *optimistic.Op
	Unstar(ref core.RepoRef) *optimistic.Op
}

// Section lists the viewer's repositories. Create one with [New].
type Section struct {
	ctx  context.Context
	svc  Service
	keys KeyMap
	feed feed.Model[core.Repo]
	now  func() time.Time

	// pinned are listed first, in order, and left out of the pages after.
	pinned []core.RepoRef
	// current is the repository the other sections show.
	current core.RepoRef
	// pending counts the changes in flight by their DoneMsg.What, so that
	// only their results reload the list.
	pending map[string]int

	styles styles
	cols   layout
	rows   map[core.RepoRef]cachedRow
}

// New returns the section, which reads repositories from svc and takes its
// keys from the configured keys. ctx bounds its requests.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		ctx:     ctx,
		svc:     svc,
		keys:    newKeyMap(keys),
		now:     time.Now,
		pending: map[string]int{},
		rows:    map[core.RepoRef]cachedRow{},
	}
	for _, opt := range opts {
		opt(s)
	}
	empty := "No repositories to show."
	if h := s.keys.Refresh.Help(); h.Key != "" {
		empty += " Press " + h.Key + " to refresh."
	}
	s.feed = feed.New(s.fetch, s.render,
		feed.WithContext(ctx),
		feed.WithKey(refKey),
		feed.WithKeyMap(s.keys.Feed),
		feed.WithEmptyText(empty),
	)
	return s
}

// listPrefix marks the cursors of the service's pages when the pinned
// repositories take the first chunk, whose cursor is empty.
const listPrefix = "list:"

// fetch adapts the service's pages to the feed. With repositories pinned,
// they are the first chunk, and the pages follow without them.
func (s *Section) fetch(ctx context.Context, cursor string) ([]core.Repo, string, error) {
	if len(s.pinned) == 0 {
		p, err := s.svc.List(ctx, reposvc.ListQuery{Cursor: cursor})
		return p.Items, p.Next, err
	}
	if cursor == "" {
		items, err := s.fetchPinned(ctx)
		return items, listPrefix, err
	}
	p, err := s.svc.List(ctx, reposvc.ListQuery{Cursor: strings.TrimPrefix(cursor, listPrefix)})
	if err != nil {
		return nil, "", err
	}
	// The page belongs to the cache, so filter into a new slice.
	items := make([]core.Repo, 0, len(p.Items))
	for i := range p.Items {
		if !s.isPinned(p.Items[i].Ref) {
			items = append(items, p.Items[i])
		}
	}
	next := ""
	if p.Next != "" {
		next = listPrefix + p.Next
	}
	return items, next, nil
}

// fetchPinned gets the pinned repositories at once. One that no longer
// exists, say after a rename, is left out rather than hiding the list.
func (s *Section) fetchPinned(ctx context.Context) ([]core.Repo, error) {
	repos := make([]core.Repo, len(s.pinned))
	errs := make([]error, len(s.pinned))
	var wg sync.WaitGroup
	for i, ref := range s.pinned {
		wg.Go(func() { repos[i], errs[i] = s.svc.Get(ctx, ref) })
	}
	wg.Wait()
	items := repos[:0]
	for i, err := range errs {
		switch {
		case err == nil:
			items = append(items, repos[i])
		case !errors.Is(err, core.ErrNotFound):
			return nil, err
		}
	}
	return items, nil
}

func (s *Section) isPinned(ref core.RepoRef) bool {
	for _, p := range s.pinned {
		if sameRef(p, ref) {
			return true
		}
	}
	return false
}

// Title returns the title of the section.
func (s *Section) Title() string {
	return Title
}

// Init fetches the first page.
func (s *Section) Init() tea.Cmd {
	return s.feed.Init()
}

// Update handles the section's keys, tracks the current repository and
// reloads after changes; everything else goes to the list.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if cmd, ok := s.press(msg); ok {
			return cmd
		}
	case ui.RepoMsg:
		s.current = msg.Repo
		return nil
	case ui.DoneMsg:
		if s.pending[msg.What] == 0 {
			return nil
		}
		s.pending[msg.What]--
		if s.pending[msg.What] == 0 {
			delete(s.pending, msg.What)
		}
		// On success the service marked the entries stale, and on failure
		// it rolled them back; either way the cache has news.
		return s.feed.Reload()
	}
	var cmd tea.Cmd
	s.feed, cmd = s.feed.Update(msg)
	return cmd
}

// press handles the section's own keys and reports whether msg was one.
func (s *Section) press(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if !s.feed.Focused() {
		return nil, false
	}
	switch {
	case key.Matches(msg, s.keys.Refresh):
		return s.feed.Reload(), true
	case key.Matches(msg, s.keys.Select):
		return s.choose(), true
	case key.Matches(msg, s.keys.Star):
		return s.toggleStar(), true
	case key.Matches(msg, s.keys.Open):
		return s.open(), true
	}
	return nil, false
}

// choose makes the selected repository current and shows its pull requests.
func (s *Section) choose() tea.Cmd {
	r, ok := s.feed.Selected()
	if !ok {
		return nil
	}
	s.current = r.Ref
	return tea.Sequence(
		func() tea.Msg { return ui.RepoMsg{Repo: r.Ref} },
		func() tea.Msg { return ui.ShowMsg{Title: pullsTitle} },
	)
}

// toggleStar stars or unstars the selected repository. The service shows
// the change in the cache at once, so the list reloads before it is sent.
func (s *Section) toggleStar() tea.Cmd {
	r, ok := s.feed.Selected()
	if !ok {
		return nil
	}
	op, what := s.svc.Star(r.Ref), "star "+r.Ref.String()
	if r.Starred {
		op, what = s.svc.Unstar(r.Ref), "unstar "+r.Ref.String()
	}
	s.pending[what]++
	return tea.Batch(s.feed.Reload(), ui.Do(s.ctx, op, what))
}

// open opens the selected repository in the browser.
func (s *Section) open() tea.Cmd {
	r, ok := s.feed.Selected()
	if !ok {
		return nil
	}
	url := r.URL
	if url == "" {
		url = "https://github.com/" + r.Ref.String()
	}
	return ui.Open(url)
}

// View renders the list.
func (s *Section) View() string {
	return s.feed.View()
}

// SetSize sets the size of the list and lays out its columns.
func (s *Section) SetSize(width, height int) {
	s.feed.SetSize(width, height)
	s.cols = newLayout(rowWidth(width), s.lead())
}

// SetTheme styles the list and its rows.
func (s *Section) SetTheme(t ui.Theme) {
	s.feed.SetStyles(t.Feed())
	s.styles = newStyles(t)
	clear(s.rows)
}

// Focus makes the section react to keys.
func (s *Section) Focus() {
	s.feed.Focus()
}

// Blur makes the section ignore keys.
func (s *Section) Blur() {
	s.feed.Blur()
}

// Help returns the keys of the section and its list.
func (s *Section) Help() help.KeyMap {
	k := s.keys
	k.Feed = s.feed.KeyMap()
	return k
}

// refKey identifies a repository regardless of case, as GitHub does.
func refKey(r core.Repo) string {
	return strings.ToLower(r.Ref.String())
}

func sameRef(a, b core.RepoRef) bool {
	return strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Name, b.Name)
}
