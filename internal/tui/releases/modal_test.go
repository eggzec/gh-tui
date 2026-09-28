package releases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
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
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No files were uploaded with it.") || strings.Contains(v, "comments") {
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
	if s := ansi.Strip(v); !strings.Contains(s, "Couldn't load the release: boom") || !strings.Contains(s, "Press r to retry, or o to open it on GitHub.") {
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

func TestKeys(t *testing.T) {
	m := newModal(&fakeService{}, 100, 30)
	// Before the release is read, o opens the page it was opened with.
	if msgs := run(t, m, press(m, "o")); len(msgs) != 1 || msgs[0] != (ui.OpenMsg{URL: "https://github.com/charmbracelet/glow/releases"}) {
		t.Errorf("o before the read = %v", msgs)
	}
	run(t, m, m.Init())
	if msgs := run(t, m, press(m, "o")); len(msgs) != 1 || msgs[0] != (ui.OpenMsg{URL: v3.URL}) {
		t.Errorf("o = %v, want the release opened on GitHub", msgs)
	}
	msgs := run(t, m, press(m, "esc"))
	if len(msgs) != 1 || msgs[0] != (ui.CloseModalMsg{Modal: m}) {
		t.Errorf("esc = %v, want the modal closed", msgs)
	}
	if m.Update(releaseMsg{id: m.id, rel: v3}) != nil {
		t.Error("a closed modal took a late read")
	}
}

// withDiagram serves v3 with a diagram in its notes.
type withDiagram struct{ fakeService }

func (f *withDiagram) Get(ctx context.Context, r core.RepoRef, id int64) (core.Release, error) {
	rel, err := f.fakeService.Get(ctx, r, id)
	rel.Body = "Notes.\n\n```mermaid\ngraph LR\n  tag --> release\n```\n"
	return rel, err
}

// A diagram in the notes links to mermaid.live, select shows its code,
// as the help says, and open still opens the release.
func TestDiagram(t *testing.T) {
	m := newModal(&withDiagram{}, 100, 30)
	run(t, m, m.Init())
	v := m.View()
	if !strings.Contains(ansi.Strip(v), "◆ flowchart · 2 lines · View diagram ↗") ||
		strings.Count(v, "\x1b]8;;https://mermaid.live/view#pako:") != 1 {
		t.Fatalf("no linked diagram:\n%q", v)
	}
	offered := false
	for _, b := range m.Help().ShortHelp() {
		offered = offered || b.Enabled() && b.Help().Desc == "diagram code"
	}
	if !offered {
		t.Error("the help doesn't offer the diagram's code")
	}
	run(t, m, m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}))
	if !strings.Contains(ansi.Strip(m.View()), "tag --> release") {
		t.Errorf("select didn't show the code:\n%s", ansi.Strip(m.View()))
	}
	if msgs := run(t, m, press(m, "o")); len(msgs) != 1 || msgs[0] != (ui.OpenMsg{URL: v3.URL}) {
		t.Errorf("o = %v, want the release opened on GitHub", msgs)
	}
}

// changedThenFails serves v3 with fewer files once, as if one was removed,
// and then fails with err.
type changedThenFails struct {
	fakeService
	fail error
}

func (f *changedThenFails) Get(context.Context, core.RepoRef, int64) (core.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.gets > 1 {
		return core.Release{}, f.fail
	}
	r := v3
	r.Assets = v3.Assets[:1]
	return r, nil
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
		{"offline", fmt.Errorf("get release: github: GET /repos/charmbracelet/glow/releases/1: %w", core.ErrOffline), "✗ Can't reach GitHub"},
		{"forbidden", fmt.Errorf("get release: github: 403 Forbidden: %w", core.ErrForbidden), "✗ You don't have access to charmbracelet/glow · o to open on GitHub"},
		{"internal", errors.New("get release: github: decode: unexpected EOF"), "✗ Something went wrong. Details are in the log"},
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
