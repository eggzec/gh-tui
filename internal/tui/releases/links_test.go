package releases

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
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
		h := m.header(width)
		links := uitest.Links(t, h)
		if len(links) < 2 || slices.ContainsFunc(links, func(l string) bool { return l != v3.URL }) {
			t.Errorf("at %d: the header links to %q, want the name and the tag to %q", width, links, v3.URL)
		}
		if strings.Contains(h, "\x1b]8;;https://evil") || strings.Contains(h, "\x1b[2J") {
			t.Errorf("at %d: the header carries the name's escapes: %q", width, h)
		}
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
