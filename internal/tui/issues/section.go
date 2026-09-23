// Package issues is the Issues section: the issues of the selected
// repository, filtered by state, each opening into a thread of comments.
package issues

import (
	"context"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
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

	// detail is the open issue's thread, shown while inDetail. It is
	// rebuilt for every issue, like the list.
	detail       thread.Model[core.Comment]
	inDetail     bool
	issue        core.Issue
	detailGen    int
	detailCtx    context.Context
	cancelDetail context.CancelFunc
	// md renders comment bodies at mdWidth.
	md      *glamour.TermRenderer
	mdWidth int

	// sent counts the changes in flight by what they did, such as
	// "close #7", to tell their DoneMsg from other sections'.
	sent map[string]int

	width, height int
	theme         ui.Theme
	rows          rowStyles
	// chips holds the rendered label chips by name and color.
	chips map[string]chip
	// cols is the layout of the rows at colsWidth.
	cols      columns
	colsWidth int

	// Rendered when what they show changes, so View only joins them.
	bar   string
	empty string
}

// New returns the Issues section, which reads issues from svc. keys maps
// action names to keys, as in the config. ctx bounds every request.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		ctx:       ctx,
		svc:       svc,
		keys:      newKeyMap(keys),
		now:       time.Now,
		filter:    core.FilterOpen,
		chips:     map[string]chip{},
		sent:      map[string]int{},
		colsWidth: -1,
	}
	for _, opt := range opts {
		opt(s)
	}
	// Assume a dark terminal until the app sets the theme.
	s.theme = ui.NewTheme(defaultPalette(), true)
	s.rows = newRowStyles(s.theme)
	s.list = s.newList()
	s.renderChrome()
	return s
}

func defaultPalette() config.Palette {
	p, _ := config.Default().Palette(true)
	return p
}

// Title implements ui.Section.
func (s *Section) Title() string { return "Issues" }

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
	if s.inDetail {
		s.detail.SetSize(s.width, s.bodyHeight())
	}
	s.renderChrome()
}

// SetTheme implements ui.Section. It builds every style the rows use.
func (s *Section) SetTheme(t ui.Theme) {
	s.theme = t
	s.rows = newRowStyles(t)
	clear(s.chips)
	s.md = nil
	s.list.SetStyles(t.Feed())
	if s.inDetail {
		s.detail.SetStyles(t.Thread())
		// The header is styled too. The thread loads whatever the new
		// layout needs on its next message, so the command can go.
		_ = s.detail.SetDocument(s.header(s.issue), s.issue.Body)
	}
	s.renderChrome()
}

// Focus implements ui.Section.
func (s *Section) Focus() {
	s.focused = true
	if s.inDetail {
		s.detail.Focus()
		return
	}
	s.list.Focus()
}

// Blur implements ui.Section.
func (s *Section) Blur() {
	s.focused = false
	s.list.Blur()
	s.detail.Blur()
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
	svc, q := s.svc, issuesvc.ListQuery{Repo: s.repo, State: s.filter}
	fetch := func(ctx context.Context, cursor string) ([]core.Issue, string, error) {
		q := q
		q.Cursor = cursor
		p, err := svc.List(ctx, q)
		return p.Items, p.Next, err
	}
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
