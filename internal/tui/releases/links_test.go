package releases

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/autolinks"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The name and the tag in the header link to the release's page, and so
// does the title of the modal once it is read, and the releases before.
func TestLinks(t *testing.T) {
	m := newModal(&fakeService{}, 80, 24)
	if got := m.Link(); got != "https://github.com/charmbracelet/glow/releases" {
		t.Errorf("before the read, the title links to %q", got)
	}
	run(t, m, m.Init())
	if got := m.Link(); got != v3.URL {
		t.Errorf("the title links to %q, want %q", got, v3.URL)
	}
	for _, width := range []int{80, 20} {
		m.rel.Name = uitest.HostileTitle
		m.SetSize(width, 24)
		links := uitest.Links(t, m.header())
		if len(links) < 1 || !slices.Contains(links, v3.URL) || slices.ContainsFunc(links, func(l string) bool { return !strings.HasPrefix(l, "https://github.com/") }) {
			t.Errorf("at %d: the header links to %q, want the page %q and others on its host", width, links, v3.URL)
		}
		if v := m.View(); strings.Contains(v, "\x1b]8;;https://evil") || strings.Contains(v, "\x1b[2J") {
			t.Errorf("at %d: the view carries the name's escapes: %q", width, v)
		}
	}
}

// The author, the tag, the commit and each contributor link to their pages on
// the host of the release, and none link where links are off or a part of
// the address is missing. The links end on their line.
func TestHeaderLinks(t *testing.T) {
	// Serial: links are a process-wide switch.
	termtext.SetLinks(true)
	t.Cleanup(func() { termtext.SetLinks(true) })
	m := newModal(&fakeService{cached: true}, 120, 30)
	m.update(extraMsg{id: m.id, extra: extra3})
	m.rel.Author = core.User{Login: "carol"}
	header := m.header()
	links := uitest.Links(t, header)
	for _, want := range []string{
		v3.URL,
		"https://github.com/carol",
		"https://github.com/charmbracelet/glow/commit/0123456789abcdef",
		"https://github.com/alice",
		"https://github.com/bob",
	} {
		if !slices.Contains(links, want) {
			t.Errorf("header lacks a link to %q: %q", want, links)
		}
	}
	if n := strings.Count(header, "\x1b]8;;https://"); n < 5 {
		t.Errorf("header opens %d links, want at least 5", n)
	}

	m.rel.URL = ""
	m.extra.TagSHA = ""
	if got := uitest.Links(t, m.header()); len(got) != 0 {
		t.Errorf("a header without a page links to %q", got)
	}

	termtext.SetLinks(false)
	m.rel.URL = v3.URL
	m.extra.TagSHA = extra3.TagSHA
	if h := m.header(); strings.Contains(h, "\x1b]8;") {
		t.Errorf("links off, but the header has one: %q", h)
	}
}

// fakeLinks records what the modal asks the autolinks service.
type fakeLinks struct {
	mu       sync.Mutex
	verified []autolinks.Candidate
}

func (f *fakeLinks) Cached(string, []autolinks.Candidate) []autolinks.Result { return nil }

func (f *fakeLinks) Verify(_ context.Context, _ string, cands []autolinks.Candidate) []autolinks.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verified = append(f.verified, cands...)
	return nil
}

func (f *fakeLinks) asked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.verified)
}

// Serial: it switches the process-wide links setting.
func TestNotesAskAboutTheirReferencesOnlyWithLinksOn(t *testing.T) {
	for _, on := range []bool{true, false} {
		f := &fakeLinks{}
		termtext.SetLinks(on)
		m := New(context.Background(), &fakeService{}, repo, v3.ID, "https://github.com/charmbracelet/glow/releases", config.Default().Keys,
			WithNow(func() time.Time { return clock }), WithLocation(time.UTC),
			WithAutolinks(ui.NewAutolinks(f, "github.com")))
		m.SetSize(100, 30)
		run(t, m, m.Init())
		if got := f.asked(); on && got == 0 {
			t.Error("links on: the references of the notes were not asked about")
		} else if !on && got != 0 {
			t.Errorf("links off: %d references asked about", got)
		}
	}
	termtext.SetLinks(true)
}
