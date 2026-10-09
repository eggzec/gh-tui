package refs

import (
	"context"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	refssvc "github.com/eggzec/gh-tui/internal/service/refs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

// The sizes inside the frame on terminals of 120 by 40 and 80 by 24.
const (
	wideW, wideH     = 92, 30
	narrowW, narrowH = 60, 18
)

var (
	repo    = core.RepoRef{Owner: "octo", Name: "gh-tui"}
	other   = core.RepoRef{Owner: "cli", Name: "go-gh"}
	secret  = core.RepoRef{Owner: "octo", Name: "secret"}
	infra   = core.RepoRef{Owner: "acme", Name: "infra"}
	self    = core.Target{Repo: repo, Number: 231, Kind: core.KindPull}
	testNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
)

// origin makes where an item was found.
func origin(group core.RefGroup, where, by string) core.RefOrigin {
	return core.RefOrigin{Group: group, Where: where, By: by, Bot: strings.HasSuffix(by, "[bot]")}
}

func closes() core.RefOrigin { return origin(core.RefClosing, "closes", "") }
func body() core.RefOrigin   { return origin(core.RefWritten, "body", "") }
func comment(by string) core.RefOrigin {
	return origin(core.RefWritten, "comment", by)
}
func mentioned(by string) core.RefOrigin { return origin(core.RefMentioned, "mentioned", by) }

// issue and pull make items that were read.
func issue(r core.RepoRef, n int, title string, state core.State, reason core.StateReason, o ...core.RefOrigin) core.Reference {
	return core.Reference{
		Target: core.Target{Repo: r, Number: n, Kind: core.KindIssue}, Title: title, State: state, Reason: reason,
		URL: "https://github.com/" + r.String() + "/issues/" + strconv.Itoa(n), Origins: o,
	}
}

func pull(r core.RepoRef, n int, title string, state core.State, draft bool, o ...core.RefOrigin) core.Reference {
	return core.Reference{
		Target: core.Target{Repo: r, Number: n, Kind: core.KindPull}, Title: title, State: state, Draft: draft,
		URL: "https://github.com/" + r.String() + "/pull/" + strconv.Itoa(n), Origins: o,
	}
}

// unreadable makes an item that can't be read.
func unreadable(r core.RepoRef, n int, why string, o ...core.RefOrigin) core.Reference {
	return core.Reference{Target: core.Target{Repo: r, Number: n}, Problem: why, Origins: o}
}

// testRefs are the links of pull request 231: two it closes, five that
// its texts write, and 47 that mention it, which are read apart.
func testRefs() core.References {
	return core.References{
		Closing: []core.Reference{
			issue(repo, 198, "Token refresh fails on GHES 3.16", core.StateOpen, "", closes(), body()),
			issue(repo, 204, "Crash when the token lacks read:org", core.StateClosed, core.ReasonCompleted, closes()),
		},
		Written: []core.Reference{
			pull(repo, 187, "Keep GraphQL entries on the shelf", core.StateMerged, false, body(), comment("bob")),
			pull(other, 412, "Expose the host's API URL", core.StateOpen, false, comment("bob")),
			pull(repo, 233, "Page the timeline", core.StateOpen, true, comment("dependabot[bot]")),
			issue(repo, 12, "Login loops on GHES", core.StateClosed, core.ReasonNotPlanned, origin(core.RefWritten, "review", "alice")),
			unreadable(secret, 9, "not found, or private", comment("carol")),
		},
		Mentioned: 47,
		ReadAt:    testNow.Add(-2 * time.Hour),
	}
}

// testMentions are the first page of the mentions of the pull request, and
// the second, which holds the rest.
func testMentions() map[string]core.Page[core.Reference] {
	first := []core.Reference{
		pull(repo, 240, "Refresh the token before it expires", core.StateOpen, false, mentioned("alice")),
		pull(repo, 239, "Read the viewer's scopes at startup", core.StateMerged, false, mentioned("bob")),
		issue(infra, 77, "GHES token rotation", core.StateClosed, core.ReasonCompleted, mentioned("carol")),
		issue(infra, 80, "Rotate the bot token weekly", core.StateOpen, "", mentioned("ci-bot[bot]")),
	}
	second := []core.Reference{
		issue(repo, 241, "Another issue that mentions this one", core.StateOpen, "", mentioned("dave")),
		issue(repo, 242, "A last issue that mentions this one", core.StateOpen, "", mentioned("erin")),
	}
	return map[string]core.Page[core.Reference]{"": {Items: first, Next: "p2"}, "p2": {Items: second}}
}

// fake serves the links from memory and counts its reads.
type fake struct {
	mu   sync.Mutex
	refs core.References
	// cached is set once the links were read, which CachedReferences then
	// serves; cachedPages holds the cursors of the pages that were.
	cached      bool
	cachedPages map[string]bool
	refsErr     error
	pages       map[string]core.Page[core.Reference]
	pagesErr    error
	// stale serves the first read of the links Stale.
	stale bool

	refReads, mentionReads, invalidated int
	queries                             []refssvc.Query
	mentionQueries                      []refssvc.MentionsQuery
}

func newFake() *fake {
	return &fake{refs: testRefs(), pages: testMentions(), cachedPages: map[string]bool{}}
}

func (f *fake) CachedReferences(refssvc.Query) (core.References, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refs, f.cached
}

func (f *fake) References(_ context.Context, q refssvc.Query) (core.References, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refReads++
	f.queries = append(f.queries, q)
	if f.refsErr != nil {
		return core.References{}, f.refsErr
	}
	f.cached = true
	r := f.refs
	if f.stale && f.refReads == 1 {
		r.Stale = true
	}
	return r, nil
}

func (f *fake) CachedMentions(q refssvc.MentionsQuery) (core.Page[core.Reference], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pages[q.Cursor], f.cachedPages[q.Cursor]
}

func (f *fake) Mentions(_ context.Context, q refssvc.MentionsQuery) (core.Page[core.Reference], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mentionReads++
	f.mentionQueries = append(f.mentionQueries, q)
	if f.pagesErr != nil {
		return core.Page[core.Reference]{}, f.pagesErr
	}
	f.cachedPages[q.Cursor] = true
	return f.pages[q.Cursor], nil
}

func (f *fake) Invalidate(core.RepoRef, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated++
	f.cached = false
	clear(f.cachedPages)
}

// counts returns how many times the links and the mentions were read.
func (f *fake) counts() (refs, mentions int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refReads, f.mentionReads
}

func (f *fake) set(r core.References) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refs = r
}

func testTheme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

// retModal is the modal the step is shown in, which a picked item replaces.
type retModal struct{ ui.Modal }

var ret = &retModal{}

// newStep returns the step of the pull request, of width by height, with
// its links read, in the ASCII icons.
func newStep(tb testing.TB, f Service, width, height int, opts ...Option) (*Step, *host) {
	tb.Helper()
	title := "Cache GraphQL reads on disk"
	opts = append([]Option{
		WithClock(func() time.Time { return testNow }), WithIcons(ui.NewIcons(config.IconsASCII)), WithReturn(ret),
		WithPageSize(4), WithItem(func() Item { return Item{Title: title, Updated: testNow} }),
	}, opts...)
	s := New(tb.Context(), f, self, true, config.Default().Keys, opts...)
	s.SetTheme(testTheme())
	s.SetSize(width, height)
	h := &host{s: s}
	h.run(s.Init())
	return s, h
}

// host runs the commands of a step the way the modal and the app would,
// feeding every message back to it, and keeps those meant for them.
type host struct {
	s   *Step
	got []tea.Msg
}

func (h *host) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil, spinner.TickMsg:
		return
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
		return
	case ui.OpenMsg, ui.NotifyMsg, ui.OpenPullMsg, ui.OpenIssueMsg, CloseMsg, ui.DoneMsg:
		h.got = append(h.got, msg)
		return
	}
	if cmds, ok := sequence(msg); ok {
		for _, c := range cmds {
			h.run(c)
		}
		return
	}
	h.run(h.s.Update(msg))
}

func (h *host) send(msg tea.Msg) { h.run(func() tea.Msg { return msg }) }

// keys presses each key, which is a key's name or, for a name that is no
// key, the text it types.
func (h *host) keys(ks ...string) {
	for _, k := range ks {
		h.run(h.s.Update(press(k)))
	}
}

// typed types text, a key for each character.
func (h *host) typed(text string) {
	for _, r := range text {
		h.keys(string(r))
	}
}

func (h *host) take() []tea.Msg {
	out := h.got
	h.got = nil
	return out
}

func sequence(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	cmds := make([]tea.Cmd, v.Len())
	for i := range cmds {
		cmds[i], _ = reflect.TypeAssert[tea.Cmd](v.Index(i))
	}
	return cmds, true
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// lines returns the view without styles, a line each with its trailing
// spaces cut.
func lines(s *Step) []string {
	ls := strings.Split(ansi.Strip(s.View()), "\n")
	for i, l := range ls {
		ls[i] = strings.TrimRight(l, " ")
	}
	return ls
}

// text is the view without styles, its spaces collapsed.
func text(s *Step) string {
	return strings.Join(strings.Fields(ansi.Strip(s.View())), " ")
}

// selected returns the text of the row under the cursor, which the view
// marks with its gutter.
func selected(s *Step) string {
	for _, l := range lines(s) {
		if rest, ok := strings.CutPrefix(l, "> "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	ls := strings.Split(v, "\n")
	if len(ls) != height {
		tb.Errorf("view has %d lines, want %d", len(ls), height)
	}
	for i, l := range ls {
		if w := ansi.StringWidth(l); w != width {
			tb.Errorf("line %d is %d wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}
