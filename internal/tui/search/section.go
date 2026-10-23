// Package search is the search modal: the query on top, a type strip above
// the Results pane that picks the kind, the counts of repositories, issues
// and pull requests at the right of Results, and a Preview pane that shows
// the selected hit. The kind on view, and the other
// kinds but code, are searched when the query is submitted; code is searched
// only when its kind is chosen or on enter, since GitHub allows few code
// searches a minute.
package search

import (
	"context"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Service is what the page needs of the search service.
type Service interface {
	CachedSearch(q search.Query) (search.Result, bool)
	Search(ctx context.Context, q search.Query) (search.Result, error)
	// Prefetch reads the first page of q.Kind into the cache.
	Prefetch(ctx context.Context, q search.Query) error
	CachedCode(q search.CodeQuery) (core.SearchPage[core.CodeHit], bool)
	Code(ctx context.Context, q search.CodeQuery) (core.SearchPage[core.CodeHit], error)
	CachedUsers(q search.UsersQuery) (core.SearchPage[core.SearchUser], bool)
	Users(ctx context.Context, q search.UsersQuery) (core.SearchPage[core.SearchUser], error)
	CachedCommits(q search.CommitsQuery) (core.SearchPage[core.SearchCommit], bool)
	Commits(ctx context.Context, q search.CommitsQuery) (core.SearchPage[core.SearchCommit], error)
	CachedTopics(q search.TopicsQuery) (core.SearchPage[core.SearchTopic], bool)
	Topics(ctx context.Context, q search.TopicsQuery) (core.SearchPage[core.SearchTopic], error)
	// Invalidate marks every cached page stale.
	Invalidate()
}

// Start returns the repositories the page offers before the user types,
// such as the viewer's own.
type Start func(ctx context.Context) ([]core.Repo, error)

// maxRecent is how many recent searches the page keeps.
const maxRecent = 8

// Option configures a Section.
type Option func(*Section)

// WithNow sets the clock that ages and the code search countdown are
// measured against. The default is time.Now.
func WithNow(now func() time.Time) Option {
	return func(s *Section) { s.now = now }
}

// WithStart offers the repositories of start while the query is empty.
func WithStart(start Start) Option {
	return func(s *Section) { s.start = start }
}

// WithHost sets the web host of the user's GitHub, with its port if it has
// one, whose pages the search modal opens. It defaults to github.com.
func WithHost(host string) Option {
	return func(s *Section) { s.host = host }
}

// WithIcons sets the glyphs that mark repositories, languages and the
// states of issues and pull requests. Without it, the icons are the
// config's default.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// WithDates sets how dates read, as ui.date_format says. The default is
// as ages.
func WithDates(d ui.Dates) Option {
	return func(s *Section) { s.dates = d }
}

// WithVoice sets how the page words what went wrong, with the keys a hint
// names and the log it points to. By default the hints name the configured
// keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(s *Section) { s.voice = v }
}

// area is the part of the page that has the focus.
type area int

const (
	inputArea area = iota
	resultsArea
)

// kinds are the kinds of results, in the order the page lists them: the
// digits 1 to len(kinds) show them, and a kind added here, with its title
// below, joins the strip.
var kinds = []core.SearchKind{core.SearchRepos, core.SearchIssues, core.SearchPulls, core.SearchCode, core.SearchUsers, core.SearchCommits, core.SearchTopics}

var kindTitles = map[core.SearchKind]string{
	core.SearchTopics:  "Topics",
	core.SearchCommits: "Commits",
	core.SearchUsers:   "Users",
	core.SearchRepos:   "Repositories",
	core.SearchIssues:  "Issues",
	core.SearchPulls:   "Pull requests",
	core.SearchCode:    "Code",
}

var lastID atomic.Int64

// Section is the search modal. Create one with New.
type Section struct {
	id    int64
	ctx   context.Context
	svc   Service
	start Start
	keys  KeyMap
	now   func() time.Time
	// host is the web host of the user's GitHub, for the links it opens.
	host string
	// voice words the errors of the results.
	voice ui.Voice

	input textinput.Model
	area  area
	// preview is whether the preview has the focus, in the results' area.
	preview bool
	// pv shows the result under the cursor, and pvKey is what it was last
	// filled with.
	pv    pager.Model
	pvKey string
	// md renders the bodies of issues for the preview, and is nil until the
	// first one is.
	md *markdown.Renderer
	// typing is whether the query takes the keys, which it does from the
	// key that starts typing until the one that stops it.
	typing  bool
	focused bool
	// comp is the candidate row of the query while it types.
	comp completion

	// text is the query the results are for, the last one submitted, and
	// textCtx bounds the reads for it, canceled when the text changes.
	text       string
	textCtx    context.Context
	cancelText context.CancelFunc
	kind       core.SearchKind

	// The other kinds are read when the query is submitted, if othersOn
	// is set, as prefetch.search.other_kinds says. othersFor is
	// the text they were read for, stopOthers stops them, and seen counts
	// them as reads ahead.
	othersOn   bool
	othersFor  string
	stopOthers context.CancelFunc
	seen       *obs.Prefetched[othersKey]
	// ahead reads the pull requests and issues of the results around the
	// cursor ahead, through reader, as prefetch.search says. prefetch is
	// the settings it starts with, and slots bound its reads with those of
	// other pages.
	prefetch *config.PrefetchLayers
	reader   details.Reader
	slots    *ui.Slots
	ahead    *ui.Aheads[details.Key]
	// previewer reads more of a hit's preview than the hit holds, for the
	// hit under the cursor and previewRows rows each side of it, as
	// prefetch.search.preview says. extras keeps what was read for text, and
	// loading the keys being read.
	previewer   PreviewReader
	previewRows int
	extras      map[string]extra
	loading     map[string]bool
	// hits holds the results of each kind for text, made when the kind is
	// first shown, and code those of code search, made when asked for.
	hits   map[core.SearchKind]*hitList
	code   *codeList
	counts map[core.SearchKind]int
	// grouping is how the repository results are grouped.
	grouping ui.Grouping[core.SearchHit]

	// codeReset is when code search resumes after GitHub ran out of
	// them, and ticking is set while the countdown to it runs.
	codeReset time.Time
	ticking   bool

	// stale holds, for each kind, the results of the query before, shown
	// dimmed until those of the new one arrive, with spin in the title.
	stale    map[core.SearchKind][]string
	spin     spinner.Model
	spinning bool

	recent []string
	starts startList

	width, height int
	theme         ui.Theme
	st            styles
	errs          ui.ErrorStyles
	icons         ui.Icons
	// dates tell when the results were updated.
	dates ui.Dates
	// langs caches the glyph of each language, by theme.
	langs map[string]string
	// links keeps the links of the rows, which are drawn on every frame.
	links termtext.Links
	view  string
}

var (
	_ ui.Section  = (*Section)(nil)
	_ ui.Capturer = (*Section)(nil)
)

// New returns the search modal, which searches through svc and binds the
// actions in keys. ctx bounds every request it makes.
func New(ctx context.Context, svc Service, keys config.Keymap, opts ...Option) *Section {
	// The other kinds are read as the defaults say until the settings say
	// otherwise.
	others := ui.Resolve(config.Default().Prefetch, "search", "other_kinds")
	s := &Section{
		id:         lastID.Add(1),
		ctx:        ctx,
		svc:        svc,
		keys:       newKeyMap(keys),
		voice:      ui.NewVoice(keys, ""),
		now:        time.Now,
		kind:       core.SearchRepos,
		othersOn:   others.Enabled,
		stopOthers: func() {},
		seen:       obs.NewPrefetched[othersKey]("search"),
		hits:       make(map[core.SearchKind]*hitList),
		grouping:   ui.Grouping[core.SearchHit]{Fields: ui.HitGroupFields()},
		stale:      make(map[core.SearchKind][]string),
		langs:      make(map[string]string),
		icons:      ui.NewIcons(config.Default().UI.Icons),
		spin:       spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	s.textCtx, s.cancelText = context.WithCancel(ctx)
	for _, opt := range opts {
		opt(s)
	}
	// Bubbles copy the voice, and read the icons through it.
	s.voice.Icons = &s.icons
	s.ahead = ui.NewAheads(ctx, "search", s.reader.Kinds("search_hit")...)
	s.ahead.Share(s.slots)
	if p := s.prefetch; p != nil {
		s.setPrefetch(*p)
	}
	s.pv = newPager(s.keys)
	s.input = textinput.New()
	s.input.Prompt = ""
	s.input.Placeholder = "Search repos, issues, pull requests, code, users, commits, topics"
	s.SetTheme(ui.NewTheme(defaultPalette(), true))
	return s
}

// Title returns the title of the page.
func (s *Section) Title() string { return ui.SearchTitle }

// Init reads the repositories offered before the user types.
func (s *Section) Init() tea.Cmd {
	return s.loadStart()
}

// Capturing reports whether the page takes every key, which it does while
// the query types, or the prompt of the results' find or filter is open.
func (s *Section) Capturing() bool {
	return s.focused && (s.typing || s.feedCapturing() || s.preview && s.pv.Capturing())
}

// Query returns the query the results are for.
func (s *Section) Query() string { return s.text }

// Typed returns the text in the query box, which the results are not for
// until it is submitted.
func (s *Section) Typed() string { return s.input.Value() }

// Kind returns the kind of results on view.
func (s *Section) Kind() core.SearchKind { return s.kind }

// SetSize sets the size of the page.
func (s *Section) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	s.layout()
	s.render()
}

// SetTheme builds the styles of the page and restyles its bubbles.
func (s *Section) SetTheme(t ui.Theme) {
	s.voice.Icons = &s.icons
	s.theme = t
	s.st = newStyles(t, s.icons)
	s.errs = t.Errors(s.icons)
	clear(s.langs)
	s.input.SetStyles(inputStyles(t))
	s.pv.SetStyles(t.Pager(s.icons))
	if s.md != nil {
		s.md.SetStyle(t.Thread(s.icons).Markdown)
		s.md.SetGlyphs(s.icons.Markdown)
	}
	s.pv.Rerender()
	s.spin.Style = t.Accent
	s.spin.Spinner = s.icons.SpinnerOr(spinner.MiniDot)
	for _, l := range s.hits {
		l.feed.SetStyles(t.Feed(s.icons))
	}
	if s.code != nil {
		s.code.feed.SetStyles(t.Feed(s.icons))
	}
	s.render()
}

// Focus puts the focus in the query, not yet typing, as the search key
// does from anywhere, or in the results when they show a query already,
// as when the page comes back into view.
func (s *Section) Focus() {
	s.focused = true
	if s.text != "" {
		s.focusArea(resultsArea)
	} else {
		s.focusArea(inputArea)
	}
	s.render()
}

// Blur makes the page ignore keys, and stops the reads ahead of the other
// kinds and of the result under the cursor, as the page leaves the screen.
func (s *Section) Blur() {
	s.focused = false
	s.leaveOthers()
	s.ahead.Reset(s.ctx)
	s.focusArea(s.area)
	s.render()
}

// View returns the page, rendered when its state last changed.
func (s *Section) View() string { return s.view }

func inputStyles(t ui.Theme) textinput.Styles {
	st := textinput.StyleState{
		Text:        t.Text,
		Placeholder: t.Subtle,
		Suggestion:  t.Subtle,
		Prompt:      lipgloss.NewStyle(),
	}
	return textinput.Styles{
		Focused: st,
		Blurred: st,
		Cursor:  textinput.CursorStyle{Color: lipgloss.Color(t.Palette.Accent), Shape: tea.CursorBlock},
	}
}

func defaultPalette() config.Palette {
	p, _ := config.Default().Palette(true)
	return p
}
