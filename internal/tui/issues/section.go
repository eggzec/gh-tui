// Package issues is the Issues section: the issues of the selected
// repository, in tabs by state and filtered in the filter modal, each
// opening into a modal with its thread of comments.
package issues

import (
	"context"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Section shows the issues of one repository. Create it with [New].
type Section struct {
	ctx  context.Context
	svc  Service
	keys keyMap
	now  func() time.Time

	repo    core.RepoRef
	hasRepo bool
	// repos reads what the viewer may do in the repositories of the
	// modals, and caps is what they may do in repo, as far as it is known.
	// viewer is the login of the signed-in user, which readViewer reads,
	// or empty until it has.
	repos      ui.Repos
	caps       core.RepoCaps
	readViewer Viewer
	viewer     string
	// tab is the state shown, and query the other filters, in GitHub's
	// search syntax; filterChips are those for the pane's title.
	tab         core.StateFilter
	query       string
	filterChips string
	started     bool
	focused     bool
	// facets offer what the filter chooses from, and milestonesRead is
	// set once the milestones of repo were read ahead.
	facets         Facets
	milestonesRead bool

	// list is rebuilt for every repository, tab and filter, so its Fetch
	// never reads the section from another goroutine.
	list       feed.Model[core.Issue]
	cancelList context.CancelFunc
	// voice words the errors of the list and of the comments.
	voice ui.Voice

	// ahead reads the issues of list and their first comments before
	// they are opened, as prefetch.issues says. rowAt returns the key of
	// row i, which is how ahead knows a row. others reads the first pages
	// of the tabs not shown, if prefetch.issues.other_tabs is on.
	// prefetch is the settings they start with, and slots bound the reads
	// of ahead with those of other pages.
	prefetch *config.PrefetchLayers
	ahead    *ui.Aheads[details.Key]
	slots    *ui.Slots
	rowAt    func(i int) (details.Key, bool)
	others   *ui.Filters[issuesvc.ListQuery]

	width, height int
	theme         ui.Theme
	rows          rowStyles
	icons         ui.Icons
	chips         chipCache
	// dates tell when the issues were updated, in the rows and the
	// modal.
	dates ui.Dates
	// avatars draws the authors' avatars in the comments of the modal.
	avatars *ui.Images
	// cols is the layout of the rows at colsWidth. room is how wide the
	// labels of the issues the list has loaded so far are; until one has
	// labels, the rows keep no room for them. A new list starts without.
	// scanned is the list's count of pages when room was last sized.
	cols      columns
	colsWidth int
	room      labelRoom
	scanned   int
	// links keeps the links of the rows, which are drawn on every frame.
	links termtext.Links

	// Rendered when what they show changes, so View only joins them. off
	// is shown in place of the list when repo has no issues.
	bar   string
	empty string
	off   string
	// hint is what the empty state tells the user to do.
	hint string
}

// New returns the Issues section, which reads issues from svc. keys maps
// action names to keys, as in the config. ctx bounds every request.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		ctx:       ctx,
		svc:       svc,
		voice:     ui.NewVoice(keys, ""),
		keys:      newKeyMap(keys),
		now:       time.Now,
		tab:       core.FilterOpen,
		colsWidth: -1,
		icons:     ui.NewIcons(config.Default().UI.Icons),
	}
	for _, opt := range opts {
		opt(s)
	}
	// Bubbles copy the voice, and read the icons through it.
	s.voice.Icons = &s.icons
	s.rowAt = func(i int) (details.Key, bool) {
		it, ok := s.list.Item(i)
		return detailKey(s.repo, it.Number), ok
	}
	s.ahead = ui.NewAheads(ctx, "issues", details.Reader{Issues: svc}.Kinds("issue")...)
	s.ahead.Share(s.slots)
	if p := s.prefetch; p != nil {
		s.setPrefetch(*p)
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

// Init loads the first page, once a repository with issues is selected,
// and reads who the viewer is.
func (s *Section) Init() tea.Cmd {
	s.started = true
	cmd := s.loadViewer()
	if !s.live() {
		return cmd
	}
	return tea.Batch(s.list.Init(), cmd)
}

// live reports whether the section shows the issues of a repository and
// reads them: it has started on one that hasn't turned its issues off.
func (s *Section) live() bool {
	return s.started && s.hasRepo && !s.issuesOff()
}

// issuesOff reports whether the repository turned its issues off, as far
// as the section knows.
func (s *Section) issuesOff() bool {
	return s.caps.Known && !s.caps.Issues
}

// SetSize implements ui.Section.
func (s *Section) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	s.list.SetSize(s.width, s.bodyHeight())
	s.renderChrome()
}

// SetTheme implements ui.Section. It builds every style the rows use.
func (s *Section) SetTheme(t ui.Theme) {
	s.voice.Icons = &s.icons
	s.theme = t
	s.rows = newRowStyles(t, s.icons)
	s.chips = newChipCache(s.rows)
	s.list.SetStyles(t.Feed(s.icons))
	// The chips may have changed width with the icons.
	s.rescanLabels()
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

// newList returns a feed of the current repository, tab and filter, sized,
// styled and focused like the section.
func (s *Section) newList() feed.Model[core.Issue] {
	if s.cancelList != nil {
		s.cancelList()
	}
	var ctx context.Context
	ctx, s.cancelList = context.WithCancel(s.ctx)
	s.ahead.Reset(ctx)
	svc, q := s.svc, s.listQuery(s.tab)
	query := func(cursor string) issuesvc.ListQuery {
		q := q
		q.Cursor = cursor
		return q
	}
	fetch := ui.FeedPages("list.issues", query, func(ctx context.Context, q issuesvc.ListQuery, again bool) (core.Page[core.Issue], error) {
		q.Again = again
		return svc.List(ctx, q)
	})
	return feed.New(fetch, s.renderRow,
		feed.WithContext(ctx),
		feed.WithKey(func(it core.Issue) string { return strconv.Itoa(it.Number) }),
		feed.WithKeyMap(s.keys.feed),
		feed.WithStyles(s.theme.Feed(s.icons)),
		feed.WithSize(s.width, s.bodyHeight()),
		feed.WithFocused(s.focused),
		feed.WithEmptyText(s.emptyText()),
		feed.WithErrorText(ui.ErrorText("load the issues", s.repo.String(), s.voice)),
	)
}

// listQuery is the query of the first page of the issues of the repository
// in state that the filter selects.
func (s *Section) listQuery(state core.StateFilter) issuesvc.ListQuery {
	return issuesvc.ListQuery{Repo: s.repo, State: state, Filter: s.query}
}

// resetList shows the list of the current repository, tab and filter from the
// top, loading it if the section has started.
func (s *Section) resetList() tea.Cmd {
	s.list = s.newList()
	// The new list has loaded no issue yet, so none with labels.
	s.room, s.scanned = labelRoom{}, 0
	s.renderChrome()
	if !s.live() {
		return nil
	}
	return s.list.Init()
}
