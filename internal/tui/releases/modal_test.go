package releases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

var (
	repo  = core.RepoRef{Owner: "charmbracelet", Name: "glow"}
	clock = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
)

// v3 is the release of the github package's fixture, in short.
var v3 = core.Release{
	ID:     368759772,
	Tag:    "v3.0.0",
	Name:   "v3.0.0",
	Author: core.User{Login: "github-actions[bot]"},
	Body: "# Glow v3\n\nThis quiet release updates [Bubble Tea](https://github.com/charmbracelet/bubbletea) " +
		"and [Lip Gloss](https://github.com/charmbracelet/lipgloss) to their glorious v2s.\n\n" +
		"## Changelog\n\n### Fixed\n* c6c5e03: fix: adaptive document(s) (#998)\n",
	URL:         "https://github.com/charmbracelet/glow/releases/tag/v3.0.0",
	PublishedAt: time.Date(2026, 8, 11, 18, 9, 45, 0, time.UTC),
	Assets: []core.ReleaseAsset{
		{Name: "checksums.txt", Size: 5300, Downloads: 12662},
		{Name: "checksums.txt.sigstore.json", Size: 10256, Downloads: 9183},
		{Name: "glow-3.0.0-1.aarch64.rpm", Size: 6200748, Downloads: 244},
	},
}

// fakeService serves v3, and counts the reads.
type fakeService struct {
	mu     sync.Mutex
	cached bool
	err    error
	gets   int

	extra       core.ReleaseExtra
	extraCached bool
	extraErr    error
	extras      int
}

func (f *fakeService) CachedGet(core.RepoRef, int64) (core.Release, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return v3, f.cached
}

func (f *fakeService) Get(context.Context, core.RepoRef, int64) (core.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.err != nil {
		return core.Release{}, f.err
	}
	f.cached = true
	return v3, nil
}

// extra3 is what the header shows beyond v3.
var extra3 = core.ReleaseExtra{
	TagSHA: "0123456789abcdef", Verified: true, AheadBy: 3, Default: "main",
	Mentions: []string{"alice", "bob", "octocat"},
}

func (f *fakeService) CachedExtra(core.RepoRef, int64) (core.ReleaseExtra, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.extra, f.extraCached
}

func (f *fakeService) Extra(context.Context, core.RepoRef, int64) (core.ReleaseExtra, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.extras++
	if f.extraErr != nil {
		return core.ReleaseExtra{}, f.extraErr
	}
	return extra3, nil
}

func (f *fakeService) AssetURL(_ context.Context, _ core.RepoRef, id int64) (string, error) {
	return fmt.Sprintf("https://assets.test/%d", id), nil
}

func newModal(svc Service, width, height int) *Modal {
	m := New(context.Background(), svc, repo, v3.ID, "https://github.com/charmbracelet/glow/releases", config.Default().Keys,
		WithNow(func() time.Time { return clock }), WithLocation(time.UTC))
	m.SetSize(width, height)
	return m
}

// run runs cmd and passes what it reports to m, until nothing is left,
// and returns the messages that m didn't take, such as those for the app.
func run(t *testing.T, m *Modal, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	var out []tea.Msg
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 && len(out) < 100 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case releaseMsg:
			queue = append(queue, m.Update(msg))
		default:
			out = append(out, msg)
		}
	}
	return out
}

func press(m *Modal, k string) tea.Cmd {
	var msg tea.KeyPressMsg
	switch k {
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
	default:
		msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
	return m.Update(msg)
}

func assertFits(t *testing.T, v string, width, height int) {
	t.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		t.Fatalf("view has %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			t.Fatalf("line %d is %d cells wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

func TestView(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{
		// Inside the frame of a 190x50 and an 80x24 terminal.
		{"190x50", 148, 38}, {"80x24", 60, 18},
	} {
		t.Run(size.name, func(t *testing.T) {
			m := newModal(&fakeService{}, size.width, size.height)
			run(t, m, m.Init())
			v := m.View()
			assertFits(t, v, size.width, size.height)
			golden.RequireEqual(t, v)
		})
	}
}

func TestOpensCachedAtOnce(t *testing.T) {
	svc := &fakeService{cached: true}
	m := newModal(svc, 100, 30)
	// Before any command runs, the cached release shows, without a
	// spinner.
	_ = m.Init()
	v := ansi.Strip(m.View())
	if strings.Contains(v, "Loading") || !strings.Contains(v, "Glow v3") || !strings.Contains(v, "checksums.txt") {
		t.Errorf("cached release opens as\n%s\nwant it shown at once", v)
	}
	if got := m.Title(); got != "v3.0.0 · charmbracelet/glow" {
		t.Errorf("Title = %q", got)
	}
}

func TestLoads(t *testing.T) {
	svc := &fakeService{}
	m := newModal(svc, 100, 30)
	if got := m.Title(); got != "Release · charmbracelet/glow" {
		t.Errorf("Title before the read = %q", got)
	}
	cmd := m.Init()
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Loading") {
		t.Errorf("uncached release opens as\n%s\nwant it loading", v)
	}
	run(t, m, cmd)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Glow v3") || strings.Contains(v, "Loading") {
		t.Errorf("read release shows as\n%s", v)
	}
	if svc.gets != 1 {
		t.Errorf("reads = %d, want 1", svc.gets)
	}
}

func TestNoAssets(t *testing.T) {
	svc := &noAssets{}
	m := newModal(svc, 100, 30)
	run(t, m, m.Init())
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No assets.") || strings.Contains(v, "comments") {
		t.Errorf("release without files shows as\n%s", v)
	}
}

type noAssets struct{ fakeService }

func (f *noAssets) Get(ctx context.Context, r core.RepoRef, id int64) (core.Release, error) {
	rel, err := f.fakeService.Get(ctx, r, id)
	rel.Assets = nil
	return rel, err
}

func (f *noAssets) CachedGet(core.RepoRef, int64) (core.Release, bool) { return core.Release{}, false }

func TestErrorIsRecoverable(t *testing.T) {
	svc := &fakeService{err: errors.New("boom")}
	m := newModal(svc, 100, 20)
	run(t, m, m.Init())
	v := m.View()
	assertFits(t, v, 100, 20)
	if s := ansi.Strip(v); !strings.Contains(s, errMark+" Something went wrong · r to retry") || strings.Contains(s, "boom") {
		t.Errorf("failed read shows as\n%s", s)
	}
	svc.mu.Lock()
	svc.err = nil
	svc.mu.Unlock()
	run(t, m, press(m, "r"))
	if s := ansi.Strip(m.View()); !strings.Contains(s, "Glow v3") {
		t.Errorf("after retry the modal shows\n%s", s)
	}
}

// A release that failed to load says why the way the user should read
// it, without the error's chain, request or status code, and names the
// open key only while there is a page to open.
func TestErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		url  string
		want string
	}{
		{"offline", fmt.Errorf("get release: github: GET /repos/charmbracelet/glow/releases/1: %w", core.ErrOffline), "u", errMark + " Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("get release: github: 403 Forbidden: %w", core.ErrForbidden), "u", errMark + " You don't have access to charmbracelet/glow"},
		{"forbidden without a page", fmt.Errorf("get release: github: 403 Forbidden: %w", core.ErrForbidden), "", errMark + " You don't have access to charmbracelet/glow"},
		{"not found", fmt.Errorf("get release: github: 404 Not Found: %w", core.ErrNotFound), "u", errMark + " This doesn't exist or is private."},
		{"internal", fmt.Errorf("get release: github: decode: %s", termtexttest.Hostile), "u", errMark + " Something went wrong · r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(context.Background(), &fakeService{err: tt.err}, repo, v3.ID, tt.url, config.Default().Keys)
			m.SetSize(100, 20)
			run(t, m, m.Init())
			termtexttest.AssertClean(t, m.View(), 100)
			v := strings.TrimRight(ansi.Strip(m.View()), " \n")
			line := ""
			for l := range strings.SplitSeq(v, "\n") {
				if strings.Contains(l, errMark) {
					line = strings.TrimSpace(l)
				}
			}
			if line != tt.want {
				t.Errorf("modal says %q, want %q", line, tt.want)
			}
			for _, leak := range []string{"github:", "get release", "GET", "403", "404", "decode"} {
				if strings.Contains(v, leak) {
					t.Errorf("modal shows %q:\n%s", leak, v)
				}
			}
		})
	}
}

// A release that failed to load says why in both themes.
func TestViewFailed(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			m := New(context.Background(), &fakeService{err: fmt.Errorf("get release: %w", core.ErrOffline)}, repo, v3.ID, "u", config.Default().Keys)
			p, err := config.Default().Palette(dark)
			if err != nil {
				t.Fatal(err)
			}
			m.SetTheme(ui.NewTheme(p, dark))
			// Inside the frame of an 80x24 terminal.
			m.SetSize(60, 18)
			run(t, m, m.Init())
			v := m.View()
			assertFits(t, v, 60, 18)
			golden.RequireEqual(t, v)
		})
	}
}

func TestKeys(t *testing.T) {
	m := newModal(&fakeService{}, 100, 30)
	run(t, m, m.Init())
	msgs := run(t, m, press(m, "esc"))
	if len(msgs) != 1 || msgs[0] != (ui.CloseModalMsg{Modal: m}) {
		t.Errorf("esc = %v, want the modal closed", msgs)
	}
	if m.Update(releaseMsg{id: m.id, rel: v3}) != nil {
		t.Error("a closed modal took a late read")
	}
}

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
}

// The layers take a key in the order the modal does: its own keys before
// the thread's, and retry only while a read failed, when the thread takes
// none.
func TestKeyLayersOrder(t *testing.T) {
	svc := &fakeService{err: errors.New("boom")}
	m := newModal(svc, 100, 30)
	run(t, m, m.Init())
	winner := func(k string) string {
		b, src, _ := uitest.Winner(m.KeyLayers(), k)
		return src + ": " + b.Help().Desc
	}
	if got := winner("r"); got != "Release: retry" {
		t.Errorf("r reaches %q after a failed read, want the retry", got)
	}
	if _, _, ok := uitest.Winner(m.KeyLayers(), "j"); ok {
		t.Error("j reaches the thread of a failed read")
	}
	svc.mu.Lock()
	svc.err = nil
	svc.mu.Unlock()
	run(t, m, press(m, "r"))
	if got := winner("j"); got != "Release: down" {
		t.Errorf("j reaches %q, want the thread", got)
	}
	if got := winner("esc"); got != "Release: close" {
		t.Errorf("esc reaches %q, want the modal's close", got)
	}
}

// The keys that do nothing while a read failed are off in the help.
func TestFailedLayerTurnsKeysOff(t *testing.T) {
	m := newModal(&fakeService{err: errors.New("boom")}, 100, 30)
	run(t, m, m.Init())
	for _, k := range []string{"o", "tab", "z", "1", "2"} {
		if b, src, ok := uitest.Winner(m.KeyLayers(), k); ok {
			t.Errorf("%s reaches %s %q after a failed read, want no key", k, src, b.Help().Desc)
		}
	}
	if msgs := run(t, m, press(m, "o")); len(msgs) != 0 {
		t.Errorf("o after a failed read = %v, want nothing", msgs)
	}
}

// Enter opens nothing in the trees, and the help says it is off.
func TestEnterOnRowDoesNothing(t *testing.T) {
	m := newModal(&fakeService{}, 100, 30)
	run(t, m, m.Init())
	if m.treeKeys.Open.Enabled() {
		t.Error("the trees' open key is on, want it off")
	}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	for _, pane := range []string{"1", "2"} {
		run(t, m, press(m, pane))
		before := m.View()
		if msgs := run(t, m, m.Update(enter)); len(msgs) != 0 {
			t.Errorf("enter in pane %s = %v, want nothing", pane, msgs)
		}
		if m.View() != before {
			t.Errorf("enter in pane %s changed the view", pane)
		}
	}
}

// The open key shows the release, or the releases page until it loads, and
// does nothing without a page.
func TestOpenOnGitHub(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		load   bool
		want   string
		wantOK bool
	}{
		{"loaded", "https://example.test/page", true, v3.URL, true},
		{"before the load", "https://example.test/page", false, "https://example.test/page", true},
		{"no page", "", false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(context.Background(), &fakeService{}, repo, v3.ID, tt.url, config.Default().Keys)
			m.SetSize(100, 30)
			if tt.load {
				run(t, m, m.Init())
			}
			msgs := run(t, m, press(m, "o"))
			if !tt.wantOK {
				if len(msgs) != 0 {
					t.Errorf("o = %v, want nothing", msgs)
				}
				return
			}
			if len(msgs) != 1 || msgs[0] != (ui.OpenMsg{URL: tt.want}) {
				t.Errorf("o = %v, want %q opened", msgs, tt.want)
			}
		})
	}
}

// TestBackspaceDoesNothingAndEscCloses checks the release has no step
// before it: the back key is taken and does nothing, and the dismiss key
// closes it, as the keys reach it through the app.
func TestBackspaceDoesNothingAndEscCloses(t *testing.T) {
	m := newModal(&fakeService{}, 100, 30)
	run(t, m, m.Init())
	if msgs := run(t, m, press(m, "backspace")); len(msgs) != 0 || m.closed {
		t.Errorf("backspace = %v, closed %v, want nothing", msgs, m.closed)
	}
	cmd, ok := m.Act(ui.ActBack)
	if !ok || cmd != nil || m.closed {
		t.Errorf("Act(back) = %v, %v, closed %v, want taken and nothing", cmd, ok, m.closed)
	}
	cmd, ok = m.Act(ui.ActDismiss)
	if msgs := run(t, m, cmd); !ok || len(msgs) != 1 || msgs[0] != (ui.CloseModalMsg{Modal: m}) {
		t.Errorf("Act(dismiss) = %v, %v, want the modal closed", msgs, ok)
	}
}

// withDiagram serves v3 with a diagram in its notes.
type withDiagram struct{ fakeService }

func (f *withDiagram) Get(ctx context.Context, r core.RepoRef, id int64) (core.Release, error) {
	rel, err := f.fakeService.Get(ctx, r, id)
	rel.Body = "Notes.\n\n```mermaid\ngraph LR\n  tag --> release\n```\n"
	return rel, err
}

// A diagram in the notes links to mermaid.live, and expand shows its code,
// as the help says.
func TestDiagram(t *testing.T) {
	m := newModal(&withDiagram{}, 100, 30)
	run(t, m, m.Init())
	v := m.View()
	if !strings.Contains(ansi.Strip(v), "◆ flowchart · 2 lines · View diagram ↗") ||
		strings.Count(v, "\x1b]8;;https://mermaid.live/view#pako:") != 1 {
		t.Fatalf("no linked diagram:\n%q", v)
	}
	// The cursor starts on the text; the diagram is the row below it.
	run(t, m, press(m, "j"))
	offered := false
	for _, b := range (ui.Hints{Layers: m.KeyLayers()}).ShortHelp() {
		offered = offered || b.Enabled() && b.Help().Desc == "expand"
	}
	if !offered {
		t.Error("the help doesn't offer to expand the diagram")
	}
	run(t, m, press(m, "l"))
	if !strings.Contains(ansi.Strip(m.View()), "tag --> release") {
		t.Errorf("select didn't show the code:\n%s", ansi.Strip(m.View()))
	}
}

// changedThenFails fails with err every time it is read, after the cached
// release showed.
type changedThenFails struct {
	fakeService
	fail error
}

func (f *changedThenFails) Get(context.Context, core.RepoRef, int64) (core.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	return core.Release{}, f.fail
}

// The files say what went wrong the way the user should read it, without
// the error's chain, request or status code, and name no key to retry,
// which the thread of files doesn't have.
func TestFilesErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"offline", fmt.Errorf("get release: github: GET /repos/charmbracelet/glow/releases/1: %w", core.ErrOffline), errMark + " Can't reach GitHub"},
		{"forbidden", fmt.Errorf("get release: github: 403 Forbidden: %w", core.ErrForbidden), errMark + " You don't have access to charmbracelet/glow"},
		{"internal", errors.New("get release: github: decode: unexpected EOF"), errMark + " Something went wrong. Details are in the log"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &changedThenFails{cached: true, fail: tt.err}
			m := New(context.Background(), svc, repo, v3.ID, "", config.Default().Keys,
				WithNow(func() time.Time { return clock }), WithVoice(ui.NewVoice(config.Default().Keys, "/var/log/gh-tui.log")))
			m.SetSize(100, 30)
			// The thread's own messages go back to the modal too.
			queue := run(t, m, m.Init())
			for len(queue) > 0 {
				msg := queue[0]
				queue = queue[1:]
				if _, tick := msg.(spinner.TickMsg); tick {
					continue
				}
				queue = append(queue, run(t, m, m.Update(msg))...)
			}
			v := ansi.Strip(m.View())
			if !strings.Contains(v, tt.want) || strings.Contains(v, "github:") || strings.Contains(v, "403") || strings.Contains(v, "/repos") || strings.Contains(v, "to retry") {
				t.Errorf("modal = %q, want %q", v, tt.want)
			}
		})
	}
}

// errMark is the error glyph of the default icons, which mark what failed.
var errMark = ui.NewIcons(config.IconsNerd).Error

// The top edge shows the day the release was published, or else drafted.
func TestReleaseDate(t *testing.T) {
	r := v3
	if got := releaseDate(r); got != r.PublishedAt.Local().Format("2006-01-02") {
		t.Errorf("published release date = %q", got)
	}
	r.PublishedAt = time.Time{}
	r.CreatedAt = time.Date(2026, 8, 1, 12, 0, 0, 0, time.Local)
	if got := releaseDate(r); got != "2026-08-01" {
		t.Errorf("draft release date = %q, want its creation day", got)
	}
	if got := releaseDate(core.Release{}); got != "" {
		t.Errorf("release without dates = %q", got)
	}
}

func pressKey(m *Modal, msg tea.KeyPressMsg) { m.Update(msg) }

// The digits and tab move the focus, and z shows the focused pane alone.
func TestPaneFocusAndZoom(t *testing.T) {
	m := newModal(&fakeService{}, 100, 30)
	run(t, m, m.Init())
	if got := m.nav.Focus(); got != paneNotes {
		t.Fatalf("first focus = %d, want the notes", got)
	}
	press(m, "2")
	if got := m.nav.Focus(); got != paneFiles {
		t.Errorf("focus after 2 = %d, want the files", got)
	}
	press(m, "1")
	if got := m.nav.Focus(); got != paneNotes {
		t.Errorf("focus after 1 = %d, want the notes", got)
	}
	pressKey(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.nav.Focus(); got != paneFiles {
		t.Errorf("focus after tab = %d, want the files", got)
	}
	if m.paneLayout().Zoomed {
		t.Fatal("zoomed before z")
	}
	press(m, "z")
	if l := m.paneLayout(); !l.Zoomed || len(l.Boxes) != 1 {
		t.Errorf("after z: zoomed=%v boxes=%d, want one zoomed box", l.Zoomed, len(l.Boxes))
	}
	assertFits(t, m.View(), 100, 30)
	press(m, "z")
	if l := m.paneLayout(); l.Zoomed || len(l.Boxes) != 2 {
		t.Errorf("after second z: zoomed=%v boxes=%d, want the stack", l.Zoomed, len(l.Boxes))
	}
}

// ctrl+up and ctrl+down move the divider between the panes, and = puts it back.
func TestPaneDividers(t *testing.T) {
	m := newModal(&fakeService{}, 100, 30)
	run(t, m, m.Init())
	notes := func() int { return m.paneLayout().Boxes[0].Rect.H }
	start := notes()
	pressKey(m, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})
	grown := notes()
	if grown == start {
		t.Fatalf("ctrl+up left the notes at %d rows", start)
	}
	pressKey(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})
	pressKey(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})
	if got := notes(); got == grown || got == start {
		t.Errorf("ctrl+down twice left the notes at %d rows (grown %d, start %d)", got, grown, start)
	}
	press(m, "=")
	if got := notes(); got != start {
		t.Errorf("= left the notes at %d rows, want %d", got, start)
	}
	assertFits(t, m.View(), 100, 30)
}

// A narrow modal shows the focused pane alone, and the digits switch it.
func TestNarrowShowsOnePane(t *testing.T) {
	w := 2*panes.MinWidth - 1
	m := newModal(&fakeService{}, w, 30)
	run(t, m, m.Init())
	if l := m.paneLayout(); !l.Zoomed || len(l.Boxes) != 1 || l.Boxes[0].Pane != paneNotes {
		t.Fatalf("narrow layout = zoomed %v, %d boxes; want the notes alone", l.Zoomed, len(l.Boxes))
	}
	press(m, "2")
	if l := m.paneLayout(); len(l.Boxes) != 1 || l.Boxes[0].Pane != paneFiles {
		t.Errorf("after 2 the layout doesn't show the files alone")
	}
	assertFits(t, m.View(), w, 30)
}

// The count of the files keeps its place on the top edge of a box too narrow
// for it and the title together: the title is cut, never the count.
func TestFilesCountKeepsRoom(t *testing.T) {
	t.Parallel()
	// Every width, so that some box is too narrow for its title and count
	// together; a narrow modal shows the files box alone.
	for w := 12; w < 2*panes.MinWidth+40; w++ {
		m := newModal(&fakeService{}, w, 30)
		run(t, m, m.Init())
		press(m, "2")
		for line := range strings.SplitSeq(ansi.Strip(m.View()), "\n") {
			if strings.Contains(line, "╭") && strings.Contains(line, "Files") && !strings.Contains(line, "3[3]") && !strings.Contains(line, " 3 ") {
				t.Errorf("width %d: the top edge %q lost the count", w, line)
			}
		}
	}
}

// With the ASCII icons the modal draws its own parts in ASCII: the title,
// the files with their downloads, and their cut names.
func TestASCIIIcons(t *testing.T) {
	m := New(context.Background(), &fakeService{}, repo, v3.ID, "https://github.com/charmbracelet/glow/releases", config.Default().Keys,
		WithNow(func() time.Time { return clock }), WithLocation(time.UTC), WithIcons(ui.NewIcons(config.IconsASCII)))
	m.SetSize(60, 18)
	run(t, m, m.Init())
	for _, s := range []string{m.Title(), ansi.Strip(m.View())} {
		// The pane frames are drawn with box-drawing borders, not icons.
		for _, r := range s {
			if r > unicode.MaxASCII && (r < 0x2500 || r > 0x257f) {
				t.Errorf("%q has the non-ASCII %q", s, r)
				break
			}
		}
	}
}
