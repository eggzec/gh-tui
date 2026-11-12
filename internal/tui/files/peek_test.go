package files

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// peeked returns the section with a preview box of 60 by 20, the cursor
// moved down onto path.
func peeked(t *testing.T, f *fake, path string, opts ...Option) *Section {
	t.Helper()
	s := loaded(t, f, 40, 20, opts...)
	s.SetPreviewSize(60, 20)
	for range 12 {
		if s.PreviewTitle() == path {
			return s
		}
		keys(s, "down")
	}
	t.Fatalf("the cursor never reached %q, at %q", path, s.PreviewTitle())
	return nil
}

// The peek draws an image the preview of the same file sent at the size of
// the peek's box, found by its repository, and names a file whose image was
// sent at another size binary, without fetching anything.
func TestPeekDrawsSentImage(t *testing.T) {
	src := &uitest.ImageHost{}
	images := uitest.Avatars(src, true)
	p := imagePreview(t, images, "logo.png")
	p.SetSize(60, 20)
	p.View()
	uitest.LoadAvatars(t, images)
	p.Update(ui.ImagesMsg{})

	asked := len(src.Asked())
	fk := sampleFake()
	png := file("logo.png", int64(len(pngBytes)))
	fk.addTree(ghTUI, "", append(slices.Clone(fk.trees[treeKey(ghTUI, "")].Entries), png)...)
	s := peeked(t, fk, "logo.png", WithImages(images))
	if n := uitest.Placeholders(t, s.PreviewView()); n != 60*19 {
		t.Errorf("%d cells show the image, want %d; view %q", n, 60*19, peekText(s))
	}

	s.SetPreviewSize(50, 20)
	if n, got := uitest.Placeholders(t, s.PreviewView()), peekText(s); n != 0 || !strings.Contains(got, "Binary file, not shown") {
		t.Errorf("at another size %d cells show the image, view %q", n, got)
	}
	if got := src.Asked(); len(got) != asked {
		t.Errorf("fetched %q, want nothing beyond the preview's %d", got, asked)
	}
}

// highlighted returns a section whose cache holds a Go file and a YAML file
// beside the sample's, with the peek of its box on path.
func highlighted(t *testing.T, path string) *Section {
	t.Helper()
	f := sampleFake()
	ci, goFile := file("ci.yml", 40), file("main.go", 60)
	f.addTree(ghTUI, "", append(slices.Clone(f.trees[treeKey(ghTUI, "")].Entries), ci, goFile)...)
	f.addBlob(ci, "name: ci\non: [push]\njobs:\n  test:\n    runs-on: ubuntu-latest\n")
	f.addBlob(goFile, mainGo)
	f.cachedBlobs[ci.SHA], f.cachedBlobs[goFile.SHA] = true, true
	return peeked(t, f, path)
}

// land runs cmd and gives each message it sends to s.
func land(s *Section, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			land(s, c)
		}
	default:
		s.Update(msg)
	}
}

// The peek runs the syntax highlight of a text file it shows, and draws the
// colours once the pager has it.
func TestPeekHighlights(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"main.go", "ci.yml"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := highlighted(t, name)
			plain := s.PreviewView()
			cmd := s.PreviewCmd()
			if cmd == nil {
				t.Fatal("no highlight queued")
			}
			if s.PreviewCmd() != nil {
				t.Error("the highlight was handed on twice")
			}
			land(s, cmd)
			if got := s.PreviewView(); got == plain || !strings.Contains(got, "\x1b[") {
				t.Errorf("view after the highlight is as before, or has no colour: %q", got)
			}
		})
	}
}

// A highlight of a file the peek has moved on from is dropped.
func TestPeekDropsStaleHighlight(t *testing.T) {
	t.Parallel()
	s := highlighted(t, "main.go")
	stale := s.PreviewCmd()
	for range 12 {
		if s.PreviewTitle() == "ci.yml" {
			break
		}
		keys(s, "up")
	}
	if s.PreviewTitle() != "ci.yml" {
		t.Fatal("cursor did not reach ci.yml")
	}
	plain := s.PreviewView()
	land(s, stale)
	if got := s.PreviewView(); got != plain {
		t.Errorf("a stale highlight changed the view: %q, want %q", got, plain)
	}
}

// A message that may have changed the cache leaves the colours of a cached
// text file alone when nothing it shows changed.
func TestPeekKeepsHighlightOnCacheMessages(t *testing.T) {
	t.Parallel()
	for name, msg := range map[string]tea.Msg{"ahead": ui.AheadMsg{}, "sync": ui.SyncMsg{}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := highlighted(t, "main.go")
			land(s, s.PreviewCmd())
			coloured := s.PreviewView()
			if !strings.Contains(coloured, "\x1b[") {
				t.Fatalf("no colour before the message: %q", coloured)
			}
			s.Update(msg)
			if got := s.PreviewView(); got != coloured {
				t.Errorf("view changed: %q, want %q", got, coloured)
			}
			if s.PreviewCmd() != nil {
				t.Error("a highlight was queued again")
			}
		})
	}
}

// Markdown is rendered at once, and queues no highlight.
func TestPeekMarkdownQueuesNothing(t *testing.T) {
	t.Parallel()
	f := sampleFake()
	f.cachedBlobs[file("AGENTS.md", 0).SHA] = true
	s := peeked(t, f, "AGENTS.md")
	if cmd := s.PreviewCmd(); cmd != nil {
		t.Errorf("markdown queued %T, want nothing", cmd())
	}
}

func peekText(s *Section) string {
	return strings.Join(strings.Fields(ansi.Strip(s.PreviewView())), " ")
}

func TestPeekShowsWhatIsCached(t *testing.T) {
	t.Parallel()
	f := sampleFake()
	f.cachedBlobs[file("AGENTS.md", 0).SHA] = true
	s := peeked(t, f, "AGENTS.md")
	if got := peekText(s); !strings.Contains(got, "Guidance for anyone.") {
		t.Errorf("preview = %q, want the cached content", got)
	}
	if n := len(f.blobSHAs()); n != 0 {
		t.Errorf("Blob called %d times, want 0", n)
	}
}

func TestPeekMissThenFill(t *testing.T) {
	t.Parallel()
	f := sampleFake()
	s := peeked(t, f, "AGENTS.md", prefetching(func(p *config.PrefetchLayers) { p.Files.Preview.Enabled = new(false) }))
	if got := peekText(s); !strings.Contains(got, "Not read yet.") {
		t.Fatalf("preview = %q, want the placeholder", got)
	}
	f.mu.Lock()
	f.cachedBlobs[file("AGENTS.md", 0).SHA] = true
	f.mu.Unlock()
	s.Update(ui.OnlineMsg{})
	if got := peekText(s); !strings.Contains(got, "Guidance for anyone.") {
		t.Errorf("preview after the fill = %q, want the content", got)
	}
	if n := len(f.blobSHAs()); n != 0 {
		t.Errorf("Blob called %d times, want 0", n)
	}
}

func TestPeekMovesAskForNothing(t *testing.T) {
	t.Parallel()
	f := sampleFake()
	s := loaded(t, f, 40, 20, prefetching(func(p *config.PrefetchLayers) { p.Enabled = false }))
	s.SetPreviewSize(60, 20)
	keys(s, "down", "down", "down", "down", "down", "down", "up", "G", "g")
	if n := len(f.blobSHAs()); n != 0 {
		t.Errorf("Blob called %d times on cursor moves, want 0", n)
	}
}

func TestPeekPlaceholders(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, want string }{
		{"go.mod", "Not read yet."},
		{"vendor-lib", "is a submodule"},
		{"cmd", "cmd"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			f := sampleFake()
			s := peeked(t, f, tc.path, prefetching(func(p *config.PrefetchLayers) { p.Enabled = false }))
			if got := peekText(s); !strings.Contains(got, tc.want) {
				t.Errorf("preview = %q, want %q", got, tc.want)
			}
			if n := len(f.blobSHAs()); n != 0 {
				t.Errorf("Blob called %d times, want 0", n)
			}
		})
	}
}

func TestPeekNotBelowOpen(t *testing.T) {
	t.Parallel()
	f := sampleFake()
	s := peeked(t, f, "AGENTS.md")
	h := newHost(s)
	h.keys("enter")
	if !strings.Contains(h.top().Title(), "AGENTS.md") {
		t.Errorf("enter opened %q, want the file", h.top().Title())
	}
}
