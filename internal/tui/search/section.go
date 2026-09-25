// Package search is the search page, laid out like github.com's: the query
// on top, the kinds of results on the left with their counts, and the
// results of the chosen kind on the right. Repositories, issues and pull
// requests are searched as the user types, in one request for all three;
// code is searched only when its kind is chosen or on enter, since GitHub
// allows few code searches a minute.
package search

import (
	"context"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Service is what the page needs of the search service.
type Service interface {
	CachedSearch(q search.Query) (search.Result, bool)
	Search(ctx context.Context, q search.Query) (search.Result, error)
	// Prefetch reads the first page of q.Kind into the cache.
	Prefetch(ctx context.Context, q search.Query) error
	CachedCode(q search.CodeQuery) (core.SearchPage[core.CodeHit], bool)
	Code(ctx context.Context, q search.CodeQuery) (core.SearchPage[core.CodeHit], error)
	// CodeLimited reports whether code search is out of requests, and when
	// it resumes.
	CodeLimited() (reset time.Time, limited bool)
	// Invalidate marks every cached page stale.
	Invalidate()
}

// Start returns the repositories the page offers before the user types,
// such as the viewer's own.
type Start func(ctx context.Context) ([]core.Repo, error)

// DefaultDebounce is how long the page waits after the last key before it
// searches.
const DefaultDebounce = 250 * time.Millisecond

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

// WithDebounce sets how long the page waits after the last key before it
// searches. The default is DefaultDebounce.
func WithDebounce(d time.Duration) Option {
	return func(s *Section) { s.debounce = max(d, 0) }
}

// WithIcons sets the glyphs that mark repositories, languages and the
// states of issues and pull requests. The default is the Nerd Font set.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// area is the part of the page that has the focus.
type area int

const (
	inputArea area = iota
	kindsArea
	resultsArea
)

// kinds are the kinds of results, in the order the page lists them.
var kinds = []core.SearchKind{core.SearchRepos, core.SearchIssues, core.SearchPulls, core.SearchCode}

var kindTitles = map[core.SearchKind]string{
	core.SearchRepos:  "Repositories",
	core.SearchIssues: "Issues",
	core.SearchPulls:  "Pull requests",
	core.SearchCode:   "Code",
}

var lastID atomic.Int64

// Section is the search page. Create one with New.
type Section struct {
	id       int64
	ctx      context.Context
	svc      Service
	start    Start
	keys     KeyMap
	now      func() time.Time
	debounce time.Duration

	input   textinput.Model
	area    area
	focused bool

	// seq counts the edits of the query; a debounce of an earlier one is
	// dropped. text is the query the results are for, and textCtx bounds
	// the reads for it, canceled when the text changes.
	seq        int
	text       string
	textCtx    context.Context
	cancelText context.CancelFunc
	kind       core.SearchKind
	// hits holds the results of each kind for text, made when the kind is
	// first shown, and code those of code search, made when asked for.
	hits   map[core.SearchKind]*hitList
	code   *codeList
	counts map[core.SearchKind]int

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
	icons         ui.Icons
	// dots caches the rendered dot of each label color, and langs the
	// glyph of each language, by theme.
	dots  map[string]string
	langs map[string]string
	view  string
}

var (
	_ ui.Section  = (*Section)(nil)
	_ ui.Capturer = (*Section)(nil)
)

// New returns the search page, which searches through svc and binds the
// actions in keys. ctx bounds every request it makes.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		id:       lastID.Add(1),
		ctx:      ctx,
		svc:      svc,
		keys:     newKeyMap(keys),
		now:      time.Now,
		debounce: DefaultDebounce,
		kind:     core.SearchRepos,
		hits:     make(map[core.SearchKind]*hitList),
		stale:    make(map[core.SearchKind][]string),
		dots:     make(map[string]string),
		langs:    make(map[string]string),
		icons:    ui.NewIcons(config.IconsNerd),
		spin:     spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	s.textCtx, s.cancelText = context.WithCancel(ctx)
	for _, opt := range opts {
		opt(s)
	}
	s.input = textinput.New()
	s.input.Prompt = ""
	s.input.Placeholder = "Search repositories, issues, pull requests and code"
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
// the query has the focus.
func (s *Section) Capturing() bool { return s.focused && s.area == inputArea }

// Query returns the query the results are for.
func (s *Section) Query() string { return s.text }

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
	s.theme = t
	s.st = newStyles(t)
	clear(s.dots)
	clear(s.langs)
	s.input.SetStyles(inputStyles(t))
	s.spin.Style = t.Accent
	for _, l := range s.hits {
		l.feed.SetStyles(t.Feed())
	}
	if s.code != nil {
		s.code.feed.SetStyles(t.Feed())
	}
	s.render()
}

// Focus puts the focus in the query, as the search key does from anywhere.
func (s *Section) Focus() {
	s.focused = true
	s.focusArea(inputArea)
	s.render()
}

// Blur makes the page ignore keys.
func (s *Section) Blur() {
	s.focused = false
	s.focusArea(s.area)
	s.render()
}

// View returns the page, rendered when its state last changed.
func (s *Section) View() string { return s.view }

// Help returns the keys of the part of the page that has the focus.
func (s *Section) Help() help.KeyMap {
	return helpKeys{k: s.keys, area: s.area, kind: s.kind, feed: s.feedKeys()}
}

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
