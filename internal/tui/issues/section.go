// Package issues is the Issues section: the issues of the selected
// repository, filtered by state, each opening into a modal with its thread
// of comments.
package issues

import (
	"context"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// Section shows the issues of one repository. Create it with [New].
type Section struct {
	ctx  context.Context
	svc  Service
	keys keyMap
	now  func() time.Time

	repo    core.RepoRef
	hasRepo bool
	filter  core.StateFilter
	started bool
	focused bool

	// list is rebuilt for every repository and filter, so its Fetch never
	// reads the section from another goroutine.
	list       feed.Model[core.Issue]
	cancelList context.CancelFunc
	// offline is marked by the list's reads when GitHub can't be reached.
	offline *ui.Offline

	// ahead reads the issues of list before they are opened, if prefetch
	// is set. rowAt returns the query of the first comments of row i,
	// which is how ahead knows a row.
	prefetch *prefetch
	ahead    *ui.Ahead[issuesvc.CommentsQuery]
	rowAt    func(i int) (issuesvc.CommentsQuery, bool)
	// others reads the first pages of the filters not shown, if
	// prefetchFilters is set.
	prefetchFilters bool
	others          *ui.Filters[issuesvc.ListQuery]

	width, height int
	theme         ui.Theme
	rows          rowStyles
	icons         ui.Icons
	chips         chipCache
	// cols is the layout of the rows at colsWidth.
	cols      columns
	colsWidth int

	// Rendered when what they show changes, so View only joins them.
	bar   string
	empty string
	// hint is what the empty state tells the user to do.
	hint string
}

// New returns the Issues section, which reads issues from svc. keys maps
// action names to keys, as in the config. ctx bounds every request.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		ctx:       ctx,
		svc:       svc,
		offline:   new(ui.Offline),
		keys:      newKeyMap(keys),
		now:       time.Now,
		filter:    core.FilterOpen,
		colsWidth: -1,
		icons:     ui.NewIcons(config.IconsNerd),
	}
	for _, opt := range opts {
		opt(s)
	}
	if p := s.prefetch; p != nil {
		s.ahead = ui.NewAhead("issue", readIssue(svc), svc.Current, p.rows, p.delay)
		s.rowAt = func(i int) (issuesvc.CommentsQuery, bool) {
			it, ok := s.list.Item(i)
			return commentsQuery(s.repo, it.Number), ok
		}
	}
	if s.prefetchFilters {
		s.others = ui.NewFilters("issue_filter", readList(svc), svc.FreshList,
			func(q issuesvc.ListQuery) string { return string(q.State) })
	}
	s.hint = "Search for a repository to see its issues."
	if k := ui.Binding(keys, config.ActionSearch, "search").Help().Key; k != "" {
		s.hint = "Press " + k + " to search for one."
	}
	// Assume a dark terminal until the app sets the theme.
	s.theme = ui.NewTheme(defaultPalette(), true)
	s.rows = newRowStyles(s.theme, s.icons)
	s.chips = newChipCache(s.rows)
	s.list = s.newList()
	s.renderChrome()
	return s
}

func defaultPalette() config.Palette {
	p, _ := config.Default().Palette(true)
	return p
}

// Title implements ui.Section.
func (s *Section) Title() string { return ui.IssuesTitle }

// Init loads the first page, once a repository is selected.
func (s *Section) Init() tea.Cmd {
	s.started = true
	if !s.hasRepo {
		return nil
	}
	return s.list.Init()
}

// SetSize implements ui.Section.
func (s *Section) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	s.list.SetSize(s.width, s.bodyHeight())
	s.renderChrome()
}

// SetTheme implements ui.Section. It builds every style the rows use.
func (s *Section) SetTheme(t ui.Theme) {
	s.theme = t
	s.rows = newRowStyles(t, s.icons)
	s.chips = newChipCache(s.rows)
	s.list.SetStyles(t.Feed())
	s.renderChrome()
}

// Focus implements ui.Section.
func (s *Section) Focus() {
	s.focused = true
	s.list.Focus()
}

// Blur implements ui.Section.
func (s *Section) Blur() {
	s.focused = false
	s.list.Blur()
}

// bodyHeight is the height under the bar.
func (s *Section) bodyHeight() int {
	return max(s.height-1, 0)
}

// newList returns a feed of the current repository and filter, sized,
// styled and focused like the section.
func (s *Section) newList() feed.Model[core.Issue] {
	if s.cancelList != nil {
		s.cancelList()
	}
	var ctx context.Context
	ctx, s.cancelList = context.WithCancel(s.ctx)
	s.ahead.Reset(ctx)
	svc, q := s.svc, s.listQuery(s.filter)
	fetch := ui.FeedPages("list.issues", s.offline, func(ctx context.Context, cursor string) (core.Page[core.Issue], error) {
		q := q
		q.Cursor = cursor
		return svc.List(ctx, q)
	})
	return feed.New(fetch, s.renderRow,
		feed.WithContext(ctx),
		feed.WithKey(func(it core.Issue) string { return strconv.Itoa(it.Number) }),
		feed.WithKeyMap(s.keys.feed),
		feed.WithStyles(s.theme.Feed()),
		feed.WithSize(s.width, s.bodyHeight()),
		feed.WithFocused(s.focused),
		feed.WithEmptyText(s.emptyText()),
	)
}

// listQuery is the query of the first page of the issues of the repository
// in filter.
func (s *Section) listQuery(filter core.StateFilter) issuesvc.ListQuery {
	return issuesvc.ListQuery{Repo: s.repo, State: filter}
}

// resetList shows the list of the current repository and filter from the
// top, loading it if the section has started.
func (s *Section) resetList() tea.Cmd {
	s.list = s.newList()
	s.renderChrome()
	if !s.started || !s.hasRepo {
		return nil
	}
	return s.list.Init()
}
