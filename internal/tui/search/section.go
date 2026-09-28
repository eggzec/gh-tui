// Package search is the search page, laid out like github.com's: the query
// on top, the kinds of results on the left with their counts, and the
// results of the chosen kind on the right. The kind on view is searched as
// the user types, and the other kinds but code once the query rests or on
// enter; code is searched only when its kind is chosen or on enter, since
// GitHub allows few code searches a minute.
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
	// Invalidate marks every cached page stale.
	Invalidate()
}

// Start returns the repositories the page offers before the user types,
// such as the viewer's own.
type Start func(ctx context.Context) ([]core.Repo, error)

// DefaultDebounce is how long the page waits after the last key before it
// searches.
const DefaultDebounce = 250 * time.Millisecond

// OthersWait is how long the query rests before the page reads the first
// pages of the kinds not on view: long enough that a pause between words
// doesn't, since each costs a search.
const OthersWait = 700 * time.Millisecond

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

// WithHost sets the web host of the user's GitHub, with its port if it has
// one, whose pages the search page opens. It defaults to github.com.
func WithHost(host string) Option {
	return func(s *Section) { s.host = host }
}

// WithIcons sets the glyphs that mark repositories, languages and the
// states of issues and pull requests. The default is the Nerd Font set.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
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
	// host is the web host of the user's GitHub, for the links it opens.
	host string
	// voice words the errors of the results.
	voice ui.Voice

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

	// edits counts the edits of the query alone, after the last of which
	// the other kinds are read once it rests for othersWait. othersFor is
	// the text they were read for, stopOthers stops them, and seen counts
	// them as reads ahead.
	edits      int
	othersWait time.Duration
	othersFor  string
	stopOthers context.CancelFunc
	seen       *obs.Prefetched[othersKey]
	// ahead reads the result under the cursor ahead, if prefetch is set.
	prefetch *prefetch
	ahead    *ui.Ahead[details.Key]
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
	errs          ui.ErrorStyles
	icons         ui.Icons
	// dots caches the rendered dot of each label color, and langs the
	// glyph of each language, by theme.
	dots  map[string]string
	langs map[string]string
	// links keeps the links of the rows, which are drawn on every frame.
	links termtext.Links
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
		id:         lastID.Add(1),
		ctx:        ctx,
		svc:        svc,
		keys:       newKeyMap(keys),
		voice:      ui.NewVoice(keys, ""),
		now:        time.Now,
		debounce:   DefaultDebounce,
		kind:       core.SearchRepos,
		othersWait: OthersWait,
		stopOthers: func() {},
		seen:       obs.NewPrefetched[othersKey]("search"),
		hits:       make(map[core.SearchKind]*hitList),
		stale:      make(map[core.SearchKind][]string),
		dots:       make(map[string]string),
		langs:      make(map[string]string),
		icons:      ui.NewIcons(config.IconsNerd),
		spin:       spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
	s.textCtx, s.cancelText = context.WithCancel(ctx)
	for _, opt := range opts {
		opt(s)
	}
	if p := s.prefetch; p != nil && p.on {
		s.ahead = details.NewAhead("search_hit", p.pulls, p.issues, 0, p.delay)
		s.ahead.Reset(ctx)
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
	// The mark is the bubbles', which draw "✗" whatever the icons.
	s.errs = t.Errors(ui.NewIcons(config.IconsUnicode))
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
