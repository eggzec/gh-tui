package search

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ Service = (*search.Service)(nil)

// now is the clock of the tests that don't count down, so ages are
// stable.
var now = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

// fakeService searches a fixed catalog the way the search service does:
// the first page of a kind comes with the count of every kind, in one
// request; a prefetch reads the first page of one kind; later pages ask
// for one kind. A result matches when its name or title contains the
// query.
type fakeService struct {
	mu      sync.Mutex
	repos   []core.SearchHit
	issues  []core.SearchHit
	pulls   []core.SearchHit
	files   []core.CodeHit
	size    int
	pages   map[string]search.Result
	code    map[string]core.SearchPage[core.CodeHit]
	counts  map[string]map[core.SearchKind]int
	limited time.Time
	// codeErr fails the next code search.
	codeErr error

	// requests counts the searches that would reach GitHub, and queries
	// what they asked for.
	requests     int
	prefetches   int
	queries      []search.Query
	codeRequests int
	invalidated  int
	// ctxs holds the context of every search and prefetch.
	ctxs []context.Context
	// hold, if set, holds every prefetch until it is closed or the
	// prefetch is canceled.
	hold chan struct{}
}

func newFake() *fakeService {
	f := &fakeService{size: search.DefaultPageSize}
	f.reset()
	f.repos, f.issues, f.pulls, f.files = catalog()
	return f
}

func (f *fakeService) reset() {
	f.pages = map[string]search.Result{}
	f.code = map[string]core.SearchPage[core.CodeHit]{}
	f.counts = map[string]map[core.SearchKind]int{}
}

func pageKey(q search.Query) string {
	return string(q.Kind) + "|" + q.Cursor + "|" + strings.ToLower(q.Text)
}

func (f *fakeService) of(k core.SearchKind) []core.SearchHit {
	switch k {
	case core.SearchIssues:
		return f.issues
	case core.SearchPulls:
		return f.pulls
	default:
		return f.repos
	}
}

func matches(text, name string) bool {
	for w := range strings.FieldsSeq(strings.ToLower(text)) {
		if strings.Contains(w, ":") {
			continue
		}
		if !strings.Contains(strings.ToLower(name), w) {
			return false
		}
	}
	return true
}

func hitName(h core.SearchHit) string {
	if h.Kind == core.SearchRepos {
		return h.Repo.Ref.String() + " " + h.Repo.Description
	}
	return h.Issue.Title
}

// page returns the page of q; call it with f.mu held.
func (f *fakeService) page(q search.Query) search.Result {
	var found []core.SearchHit
	all := f.of(q.Kind)
	for i := range all {
		if matches(q.Text, hitName(all[i])) {
			found = append(found, all[i])
		}
	}
	start, _ := strconv.Atoi(q.Cursor)
	start = min(start, len(found))
	end := min(start+f.size, len(found))
	var r search.Result
	r.Items = slices.Clone(found[start:end])
	r.Total = len(found)
	if end < len(found) {
		r.Next = strconv.Itoa(end)
	}
	return r
}

func (f *fakeService) CachedSearch(q search.Query) (search.Result, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if q.Kind == core.SearchAll {
		q.Kind = core.SearchRepos
	}
	r, ok := f.pages[pageKey(q)]
	r.Counts = maps.Clone(f.counts[strings.ToLower(q.Text)])
	return r, ok
}

func (f *fakeService) Search(ctx context.Context, q search.Query) (search.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxs = append(f.ctxs, ctx)
	if strings.Contains(q.Text, "is:bogus") {
		f.requests++
		return search.Result{}, fmt.Errorf("search %q: %w", q.Text, &core.InvalidQueryError{Reason: "is:bogus is not a valid qualifier"})
	}
	if r, ok := f.pages[pageKey(q)]; ok {
		r.Counts = maps.Clone(f.counts[strings.ToLower(q.Text)])
		return r, nil
	}
	f.requests++
	f.queries = append(f.queries, q)
	text := strings.ToLower(q.Text)
	if f.counts[text] == nil {
		f.counts[text] = map[core.SearchKind]int{}
	}
	if q.Cursor == "" {
		for _, k := range []core.SearchKind{core.SearchRepos, core.SearchIssues, core.SearchPulls} {
			other := q
			other.Kind = k
			f.counts[text][k] = f.page(other).Total
		}
	}
	f.pages[pageKey(q)] = f.page(q)
	r := f.pages[pageKey(q)]
	r.Counts = maps.Clone(f.counts[text])
	return r, nil
}

func (f *fakeService) Prefetch(ctx context.Context, q search.Query) error {
	f.mu.Lock()
	f.ctxs = append(f.ctxs, ctx)
	hold := f.hold
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(q.Text, "is:bogus") {
		return &core.InvalidQueryError{Reason: "is:bogus is not a valid qualifier"}
	}
	if _, ok := f.pages[pageKey(q)]; ok {
		return nil
	}
	f.prefetches++
	p := f.page(q)
	f.pages[pageKey(q)] = p
	text := strings.ToLower(q.Text)
	if f.counts[text] == nil {
		f.counts[text] = map[core.SearchKind]int{}
	}
	f.counts[text][q.Kind] = p.Total
	return nil
}

func (f *fakeService) fetches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests + f.prefetches
}

func codeKey(q search.CodeQuery) string { return q.Cursor + "|" + strings.ToLower(q.Text) }

func (f *fakeService) CachedCode(q search.CodeQuery) (core.SearchPage[core.CodeHit], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.code[codeKey(q)]
	return p, ok
}

func (f *fakeService) Code(_ context.Context, q search.CodeQuery) (core.SearchPage[core.CodeHit], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.code[codeKey(q)]; ok {
		return p, nil
	}
	if time.Now().Before(f.limited) {
		return core.SearchPage[core.CodeHit]{}, &core.RateLimitError{Reset: f.limited}
	}
	f.codeRequests++
	if err := f.codeErr; err != nil {
		f.codeErr = nil
		return core.SearchPage[core.CodeHit]{}, err
	}
	var p core.SearchPage[core.CodeHit]
	for _, h := range f.files {
		if matches(q.Text, h.Path+" "+h.Fragments[0].Text) {
			p.Items = append(p.Items, h)
		}
	}
	p.Total = len(p.Items)
	f.code[codeKey(q)] = p
	text := strings.ToLower(q.Text)
	if f.counts[text] == nil {
		f.counts[text] = map[core.SearchKind]int{}
	}
	f.counts[text][core.SearchCode] = p.Total
	return p, nil
}

func (f *fakeService) Invalidate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated++
	f.reset()
}

func (f *fakeService) stats() (requests, code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.codeRequests
}

func repo(owner, name, desc, lang string, stars int, ago time.Duration) core.SearchHit {
	ref := core.RepoRef{Owner: owner, Name: name}
	return core.SearchHit{Kind: core.SearchRepos, Repo: core.Repo{
		Ref: ref, Description: desc, Language: lang, Stars: stars, UpdatedAt: now.Add(-ago),
		URL: "https://github.com/" + ref.String(),
	}}
}

func issue(kind core.SearchKind, repo string, n int, title string, state core.State, draft bool, labels ...core.Label) core.SearchHit {
	r, _ := core.ParseRepoRef(repo)
	path := "issues"
	if kind == core.SearchPulls {
		path = "pull"
	}
	return core.SearchHit{Kind: kind, Draft: draft, Issue: core.Issue{
		Repo: r, Number: n, Title: title, State: state, Labels: labels, Comments: n % 7,
		Author: core.User{Login: "octocat"}, UpdatedAt: now.Add(-time.Duration(n) * time.Hour),
		URL: fmt.Sprintf("https://github.com/%s/%s/%d", repo, path, n),
	}}
}

// catalog is what the fake searches: 45 repositories about terminals, a
// few issues and pull requests, and two files.
func catalog() (repos, issues, pulls []core.SearchHit, files []core.CodeHit) {
	repos = make([]core.SearchHit, 0, 45)
	repos = append(repos,
		repo("charmbracelet", "bubbletea", "A powerful little TUI framework", "Go", 31000, 3*time.Hour),
		repo("charmbracelet", "lipgloss", "Style definitions for nice terminal layouts", "Go", 9000, 30*time.Hour),
		repo("cli", "cli", "GitHub's official command line tool", "Go", 38000, 2*time.Hour),
	)
	for i := range 42 {
		repos = append(repos, repo("octo", fmt.Sprintf("tea-%02d", i), "Another terminal tool", "Rust", i, time.Duration(i)*24*time.Hour))
	}
	bug := core.Label{Name: "bug", Color: "d73a4a"}
	help := core.Label{Name: "help wanted", Color: "008672"}
	issues = []core.SearchHit{
		issue(core.SearchIssues, "charmbracelet/bubbletea", 1203, "Terminal tea renders twice after resize", core.StateOpen, false, bug, help),
		issue(core.SearchIssues, "cli/cli", 9001, "Tea time: extensions that render their own TUI", core.StateClosed, false),
	}
	pulls = []core.SearchHit{
		issue(core.SearchPulls, "charmbracelet/bubbletea", 1402, "Tea: render only the cells that changed", core.StateOpen, false),
		issue(core.SearchPulls, "charmbracelet/bubbletea", 1388, "Tea: add a cursor to the textarea", core.StateMerged, false),
		issue(core.SearchPulls, "cli/cli", 9100, "Draft: tea support in gh", core.StateOpen, true),
	}
	text := "package tea\n\n// Program is a terminal user interface.\ntype Program struct {\n\tinput io.Reader\n}\n"
	files = []core.CodeHit{
		{
			Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}, Path: "tea.go", SHA: "b1",
			URL:       "https://github.com/charmbracelet/bubbletea/blob/main/tea.go",
			Fragments: []core.Fragment{{Text: text, Matches: [][2]int{{8, 11}, {42, 50}}}},
		},
		{
			Repo: core.RepoRef{Owner: "cli", Name: "cli"}, Path: "pkg/cmd/tea/tea.go", SHA: "b2",
			URL:       "https://github.com/cli/cli/blob/trunk/pkg/cmd/tea/tea.go",
			Fragments: []core.Fragment{{Text: "func NewCmdTea() *cobra.Command {\n\treturn nil\n}", Matches: [][2]int{{10, 13}}}},
		},
	}
	return repos, issues, pulls, files
}

func startRepos(context.Context) ([]core.Repo, error) {
	return []core.Repo{
		{Ref: core.RepoRef{Owner: "octocat", Name: "hello-world"}, Description: "My first repository"},
		{Ref: core.RepoRef{Owner: "octocat", Name: "dotfiles"}},
	}, nil
}

// withOthersWait sets how long the query rests before the other kinds are
// read. The default is OthersWait.
func withOthersWait(d time.Duration) Option {
	return func(s *Section) { s.othersWait = d }
}

// newSection returns a focused page of width by height over svc, whose
// clock is now and which searches on every key, and reads the other kinds
// at once, unless opts say otherwise.
func newSection(tb testing.TB, svc Service, width, height int, opts ...Option) *Section {
	tb.Helper()
	opts = append([]Option{WithNow(func() time.Time { return now }), WithStart(startRepos), WithDebounce(0), withOthersWait(0)}, opts...)
	s := New(tb.Context(), svc, config.Default().Keys, opts...)
	p, err := config.Default().Palette(true)
	if err != nil {
		tb.Fatal(err)
	}
	s.SetTheme(ui.NewTheme(p, true))
	s.SetSize(width, height)
	s.Focus()
	run(tb, s, s.Init())
	return s
}

// run executes cmd and gives every resulting message to s, the way the app
// would, until no commands are left, and returns the messages meant for
// the app. Spinner ticks and the ticks of the countdown are dropped;
// tests of the countdown run in a synctest bubble, where the ticks don't
// sleep, and send them themselves.
func run(tb testing.TB, s *Section, cmd tea.Cmd) []tea.Msg {
	tb.Helper()
	app, _ := drive(tb, s, cmd, nil, nil)
	return app
}

// drive is run, except that the messages hold reports true for are kept
// from s and appended to held, for the caller to give it later.
func drive(tb testing.TB, s *Section, cmd tea.Cmd, hold func(tea.Msg) bool, held []tea.Msg) (app, _ []tea.Msg) {
	tb.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if cmds, ok := sequence(msg); ok {
			queue = append(queue, cmds...)
			continue
		}
		switch msg := msg.(type) {
		case nil, spinner.TickMsg, codeTickMsg:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case ui.OpenMsg, ui.NotifyMsg, ui.RepoMsg, ui.OpenPullMsg, ui.OpenIssueMsg, ui.OpenFileMsg, ui.BackMsg:
			app = append(app, msg)
		default:
			if hold != nil && hold(msg) {
				held = append(held, msg)
				continue
			}
			queue = append(queue, s.Update(msg))
		}
	}
	return app, held
}

// sequence unpacks the message of tea.Sequence, whose type is unexported.
func sequence(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	cmds := make([]tea.Cmd, v.Len())
	for i := range cmds {
		cmds[i] = v.Index(i).Interface().(tea.Cmd)
	}
	return cmds, true
}

// press presses each key and runs the resulting commands.
func press(tb testing.TB, s *Section, keys ...string) []tea.Msg {
	tb.Helper()
	var app []tea.Msg //nolint:prealloc // Most keys send nothing to the app.
	for _, k := range keys {
		app = append(app, run(tb, s, s.Update(keyPress(k)))...)
	}
	return app
}

// typeText types text into the query, a key at a time.
func typeText(tb testing.TB, s *Section, text string) {
	tb.Helper()
	for _, r := range text {
		press(tb, s, string(r))
	}
}

func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+o":
		return tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// errMark is the error glyph of the default icons, which mark what failed.
var errMark = ui.NewIcons(config.IconsNerd).Error
