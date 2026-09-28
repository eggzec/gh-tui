// Package pulls is the Pull requests section: a list of the pull requests of
// the selected repository, in tabs by state and filtered in the filter
// modal, each opening into a modal with its detail and comments.
package pulls

import (
	"cmp"
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Service is what the section needs of the pull requests service.
type Service interface {
	List(ctx context.Context, q pulls.ListQuery) (core.Page[core.PullRequest], error)
	// FreshList reports whether List returns the page of q without a
	// request. It may do I/O.
	FreshList(q pulls.ListQuery) bool
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
	// rawKeys are the configured keys, for the steps of the modal.
	rawKeys map[string][]string
	now     func() time.Time
	// checks reads the checks of the modal's Checks step, and checksOpts
	// configure it. Without checks, the modal has no such step.
	checks     checks.Service
	checksOpts []checks.Option
	// mergeMethod is how merge merges, if the repository allows it.
	mergeMethod core.MergeMethod
	// repos reads what the viewer may do in the repositories of the
	// modals, and caps is what they may do in repo, as far as it is known.
	repos ui.Repos
	caps  core.RepoCaps

	repo    core.RepoRef
	hasRepo bool
	// tab is the state shown, or empty for all of them, and query the
	// other filters, in GitHub's search syntax; chips are those for the
	// pane's title.
	tab     core.State
	query   string
	chips   string
	facets  Facets
	started bool
	focused bool

	// feed lists the pull requests of repo in tab that query selects. It is nil until the
	// section has started with a repository.
	feed       *feed.Model[core.PullRequest]
	cancelFeed context.CancelFunc
	// offline is marked by the feed's reads when GitHub can't be reached.
	offline *ui.Offline
	// voice words the errors of the feed and of the comments.
	voice ui.Voice

	// ahead reads the details of the rows of feed before they are opened,
	// if prefetch is set. rowAt returns the query of the first comments of
	// row i, which is how ahead knows a row.
	prefetch *prefetch
	ahead    *ui.Ahead[pulls.CommentsQuery]
	rowAt    func(i int) (pulls.CommentsQuery, bool)
	// others reads the first pages of the tabs not shown, if
	// prefetchFilters is set.
	prefetchFilters bool
	others          *ui.Filters[pulls.ListQuery]

	width, height int
	theme         ui.Theme
	st            styles
	icons         ui.Icons
	cols          columns
	// links keeps the links of the rows, which are drawn on every frame.
	links  termtext.Links
	header string
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

// WithMergeMethod sets how pull requests are merged, in the repositories
// that allow it; the others merge as the viewer did last, or as they
// allow. The default is core.MergeSquash.
func WithMergeMethod(m core.MergeMethod) Option {
	return func(s *Section) { s.mergeMethod = m }
}

// WithRepos reads what the viewer may do in the repository of a modal
// from r, when it isn't the selected one, whose caps the app sends in a
// ui.CapsMsg. Until they are known, every change is offered, and GitHub
// refuses what it doesn't allow.
func WithRepos(r ui.Repos) Option {
	return func(s *Section) { s.repos = r }
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

// WithVoice sets how the section words what went wrong, with the keys a
// hint names and the log it points to. By default the hints name the
// configured keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(s *Section) { s.voice = v }
}

// WithIcons sets the glyphs of the states of pull requests. The default is
// the Nerd Font set.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// WithChecks shows the checks of a pull request in a step of its modal,
// which the checks key opens there and on the rows of the list, read from
// svc and configured by opts. The default has no such step.
func WithChecks(svc checks.Service, opts ...checks.Option) Option {
	return func(s *Section) { s.checks, s.checksOpts = svc, opts }
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

// WithFilterPrefetch reads the first page of each state not shown once the
// user switched tabs in a repository, with the next or previous tab key,
// and the list shown loaded, so that switching further shows them at once.
// It reads them once per repository and session. Each costs a request;
// pages cached fresh are skipped, and so is a list the user filtered. The
// default reads nothing ahead.
func WithFilterPrefetch() Option {
	return func(s *Section) { s.prefetchFilters = true }
}

// New returns the section, reading from svc with the configured keys. ctx
// bounds every request it makes.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		ctx:         ctx,
		svc:         svc,
		offline:     new(ui.Offline),
		voice:       ui.NewVoice(keys, ""),
		keys:        newKeyMap(keys),
		rawKeys:     keys,
		now:         time.Now,
		mergeMethod: core.MergeSquash,
		tab:         core.StateOpen,
		icons:       ui.NewIcons(config.IconsNerd),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.keys.Checks.SetEnabled(s.keys.Checks.Enabled() && s.checks != nil)
	if p := s.prefetch; p != nil {
		s.ahead = ui.NewAhead("pull", readDetail(svc), svc.Current, p.rows, p.delay)
		s.rowAt = func(i int) (pulls.CommentsQuery, bool) {
			pr, ok := s.feed.Item(i)
			return commentsQuery(s.repo, pr.Number), ok
		}
	}
	if s.prefetchFilters {
		s.others = ui.NewFilters("pull_filter", readList(svc), svc.FreshList,
			func(q pulls.ListQuery) string { return cmp.Or(string(q.State), "all") })
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

// newFeed replaces the feed with one for the current repository, tab and
// filter, and returns the command that loads it.
func (s *Section) newFeed() tea.Cmd {
	if s.cancelFeed != nil {
		s.cancelFeed()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.ahead.Reset(ctx)
	q := s.listQuery(s.tab)
	svc := s.svc
	query := func(cursor string) pulls.ListQuery {
		q := q
		q.Cursor = cursor
		return q
	}
	fetch := ui.FeedPages("list.pulls", s.offline, query, func(ctx context.Context, q pulls.ListQuery, again bool) (core.Page[core.PullRequest], error) {
		q.Again = again
		return svc.List(ctx, q)
	})
	f := feed.New(fetch, s.renderRow,
		feed.WithContext(ctx),
		feed.WithKey(pullKey),
		feed.WithKeyMap(s.keys.feed),
		feed.WithStyles(s.theme.Feed()),
		feed.WithFocused(s.focused),
		feed.WithEmptyText(s.emptyText()),
		feed.WithErrorText(ui.ErrorText("load the pull requests", s.repo.String(), s.voice)),
	)
	s.feed, s.cancelFeed = &f, cancel
	s.layout()
	s.renderHeader()
	return f.Init()
}

// listQuery is the query of the first page of the pull requests of the
// repository in state that the filter selects.
func (s *Section) listQuery(state core.State) pulls.ListQuery {
	return pulls.ListQuery{Repo: s.repo, State: state, Filter: s.query}
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
