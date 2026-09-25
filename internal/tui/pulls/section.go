// Package pulls is the Pull requests section: a list of the pull requests of
// the selected repository, filtered by state, each opening into a modal with
// its detail and comments.
package pulls

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// Service is what the section needs of the pull requests service.
type Service interface {
	List(ctx context.Context, q pulls.ListQuery) (core.Page[core.PullRequest], error)
	CachedGet(repo core.RepoRef, number int) (core.PullRequestDetail, bool)
	Get(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	CachedComments(q pulls.CommentsQuery) (core.Page[core.Comment], bool)
	Comments(ctx context.Context, q pulls.CommentsQuery) (core.Page[core.Comment], error)
	// Current reports whether the detail of the pull request of q and the
	// comments q selects are cached so that reading them costs no request.
	Current(q pulls.CommentsQuery) bool
	// Invalidate marks what is cached of repo stale, so that the reads
	// after it ask GitHub.
	Invalidate(repo core.RepoRef)

	// The changes are shown in the cache at once. The returned Op sends
	// them.
	Merge(repo core.RepoRef, number int, method core.MergeMethod) *optimistic.Op
	Close(repo core.RepoRef, number int) *optimistic.Op
	Reopen(repo core.RepoRef, number int) *optimistic.Op
	MarkReady(repo core.RepoRef, number int) *optimistic.Op
	ConvertToDraft(repo core.RepoRef, number int) *optimistic.Op
}

// Section shows the pull requests of the selected repository. Create it with
// [New].
type Section struct {
	ctx  context.Context
	svc  Service
	keys keyMap
	now  func() time.Time
	// mergeMethod is how merge merges.
	mergeMethod core.MergeMethod

	repo    core.RepoRef
	hasRepo bool
	filter  core.State
	started bool
	focused bool

	// feed lists the pull requests of repo in filter. It is nil until the
	// section has started with a repository.
	feed       *feed.Model[core.PullRequest]
	cancelFeed context.CancelFunc
	// offline is marked by the feed's reads when GitHub can't be reached.
	offline *ui.Offline

	// ahead reads the details of the rows of feed before they are opened,
	// if prefetch is set. rowAt returns the query of the first comments of
	// row i, which is how ahead knows a row.
	prefetch *prefetch
	ahead    *ui.Ahead[pulls.CommentsQuery]
	rowAt    func(i int) (pulls.CommentsQuery, bool)

	width, height int
	theme         ui.Theme
	st            styles
	icons         ui.Icons
	cols          columns
	header        string
	// blank is the empty state shown until a repository is picked, and
	// hint what it tells the user to do.
	blank string
	hint  string
}

// Option configures a Section.
type Option func(*Section)

// WithClock sets the function that tells the time, from which ages such as
// "3d" are counted. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Section) { s.now = now }
}

// WithMergeMethod sets how pull requests are merged. The default is
// core.MergeSquash.
func WithMergeMethod(m core.MergeMethod) Option {
	return func(s *Section) { s.mergeMethod = m }
}

// WithOffline shares off with other sections, so that the user is told once
// for all of them that GitHub can't be reached. By default the section has
// its own.
func WithOffline(off *ui.Offline) Option {
	return func(s *Section) {
		if off != nil {
			s.offline = off
		}
	}
}

// WithIcons sets the glyphs of the states of pull requests. The default is
// the Nerd Font set.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// prefetch is how the details are read ahead.
type prefetch struct {
	rows  int
	delay time.Duration
}

// WithPrefetch reads the detail and the first comments of the first rows
// of each list once it loads, and of the row under the cursor once the
// cursor has rested on it for delay, so that they open at once. Each costs
// two requests; details already cached are skipped. The default reads
// nothing ahead.
func WithPrefetch(rows int, delay time.Duration) Option {
	return func(s *Section) { s.prefetch = &prefetch{rows: rows, delay: delay} }
}

// New returns the section, reading from svc with the configured keys. ctx
// bounds every request it makes.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		ctx:         ctx,
		svc:         svc,
		offline:     new(ui.Offline),
		keys:        newKeyMap(keys),
		now:         time.Now,
		mergeMethod: core.MergeSquash,
		filter:      core.StateOpen,
		icons:       ui.NewIcons(config.IconsNerd),
	}
	for _, opt := range opts {
		opt(s)
	}
	if p := s.prefetch; p != nil {
		s.ahead = ui.NewAhead("pull", readDetail(svc), svc.Current, p.rows, p.delay)
		s.rowAt = func(i int) (pulls.CommentsQuery, bool) {
			pr, ok := s.feed.Item(i)
			return commentsQuery(s.repo, pr.Number), ok
		}
	}
	s.hint = "Search for a repository to see its pull requests."
	if k := ui.Binding(keys, config.ActionSearch, "search").Help().Key; k != "" {
		s.hint = "Press " + k + " to search for one."
	}
	// The app sets the theme of the terminal soon after; until then assume
	// a dark one.
	p, _ := config.Default().Palette(true)
	s.SetTheme(ui.NewTheme(p, true))
	return s
}

// Title implements ui.Section.
func (s *Section) Title() string { return ui.PullsTitle }

// Init lists the pull requests of the repository, if one is selected.
func (s *Section) Init() tea.Cmd {
	s.started = true
	if !s.hasRepo {
		return nil
	}
	return s.newFeed()
}

// newFeed replaces the feed with one for the current repository and filter,
// and returns the command that loads it.
func (s *Section) newFeed() tea.Cmd {
	if s.cancelFeed != nil {
		s.cancelFeed()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.ahead.Reset(ctx)
	q := pulls.ListQuery{Repo: s.repo, State: s.filter}
	svc := s.svc
	fetch := ui.FeedPages("list.pulls", s.offline, func(ctx context.Context, cursor string) (core.Page[core.PullRequest], error) {
		q := q
		q.Cursor = cursor
		return svc.List(ctx, q)
	})
	f := feed.New(fetch, s.renderRow,
		feed.WithContext(ctx),
		feed.WithKey(pullKey),
		feed.WithKeyMap(s.keys.feed),
		feed.WithStyles(s.theme.Feed()),
		feed.WithFocused(s.focused),
		feed.WithEmptyText(s.emptyText()),
	)
	s.feed, s.cancelFeed = &f, cancel
	s.layout()
	s.renderHeader()
	return f.Init()
}

// SetSize implements ui.Section.
func (s *Section) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	s.layout()
	s.renderHeader()
	s.blank = s.st.noRepo(s.width, s.height, s.hint)
}

func (s *Section) layout() {
	if s.feed != nil {
		s.feed.SetSize(s.width, max(s.height-headerHeight, 0))
	}
}

// SetTheme implements ui.Section.
func (s *Section) SetTheme(t ui.Theme) {
	s.theme = t
	s.st = newStyles(t, s.icons)
	if s.feed != nil {
		s.feed.SetStyles(t.Feed())
	}
	s.renderHeader()
	s.blank = s.st.noRepo(s.width, s.height, s.hint)
}

// Focus implements ui.Section.
func (s *Section) Focus() {
	s.focused = true
	if s.feed != nil {
		s.feed.Focus()
	}
}

// Blur implements ui.Section.
func (s *Section) Blur() {
	s.focused = false
	if s.feed != nil {
		s.feed.Blur()
	}
}

// View implements ui.Section.
func (s *Section) View() string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	if !s.hasRepo || s.feed == nil {
		return s.blank
	}
	if s.height <= headerHeight {
		return s.header
	}
	return s.header + "\n" + s.feed.View()
}
