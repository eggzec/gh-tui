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

	width, height int
	theme         ui.Theme
	st            styles
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

// New returns the section, reading from svc with the configured keys. ctx
// bounds every request it makes.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		ctx:         ctx,
		svc:         svc,
		keys:        newKeyMap(keys),
		now:         time.Now,
		mergeMethod: core.MergeSquash,
		filter:      core.StateOpen,
	}
	for _, opt := range opts {
		opt(s)
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
	q := pulls.ListQuery{Repo: s.repo, State: s.filter}
	svc := s.svc
	fetch := func(ctx context.Context, cursor string) ([]core.PullRequest, string, error) {
		q := q
		q.Cursor = cursor
		p, err := svc.List(ctx, q)
		return p.Items, p.Next, err
	}
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
	s.st = newStyles(t)
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
