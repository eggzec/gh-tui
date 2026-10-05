package files

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

func TestMarkdownFile(t *testing.T) {
	for name, want := range map[string]bool{
		"README.md":          true,
		"docs/Guide.MD":      true,
		"notes.markdown":     true,
		"README":             true,
		"sub/readme":         true,
		"README.txt":         false,
		"READMEFIRST":        false,
		"main.go":            false,
		"markdown":           false,
		"docs/md":            false,
		"CHANGELOG.mdx":      false,
		".md":                true,
		"archive.md.tar.gz":  false,
		"dir.md/inside.json": false,
	} {
		if got := markdownFile(name); got != want {
			t.Errorf("markdownFile(%q) = %v, want %v", name, got, want)
		}
	}
}

// sourced returns the top modal of h as a ui.Sourced.
func sourced(t *testing.T, h *host) ui.Sourced {
	t.Helper()
	s, ok := h.top().(ui.Sourced)
	if !ok {
		t.Fatalf("the top modal %T shows no source", h.top())
	}
	return s
}

func TestPreviewRendersMarkdown(t *testing.T) {
	h := openRow(t, sampleFake(), rowAgents)
	v := h.modal()
	if strings.Contains(v, "# AGENTS.md") || !strings.Contains(v, "AGENTS.md") || !strings.Contains(v, "Guidance for anyone.") {
		t.Errorf("preview isn't rendered:\n%s", v)
	}
	if raw, ok := sourced(t, h).Raw(); raw || !ok {
		t.Errorf("Raw() = %v, %v, want false, true", raw, ok)
	}
	// The status line counts the rendered lines.
	if !strings.Contains(v, "line 1/3") {
		t.Errorf("status line doesn't count the rendered lines:\n%s", v)
	}
}

func TestPreviewRawOnAndOff(t *testing.T) {
	h := openRow(t, sampleFake(), rowAgents)
	s := sourced(t, h)
	h.run(s.SetRaw(true))
	if v := h.modal(); !strings.Contains(v, "# AGENTS.md") {
		t.Errorf("raw on doesn't show the source:\n%s", v)
	}
	if raw, ok := s.Raw(); !raw || !ok {
		t.Errorf("Raw() after raw on = %v, %v, want true, true", raw, ok)
	}
	// Again changes nothing.
	if cmd := s.SetRaw(true); cmd != nil {
		t.Error("raw on twice did something")
	}
	h.run(s.SetRaw(false))
	if v := h.modal(); strings.Contains(v, "# AGENTS.md") {
		t.Errorf("raw off doesn't render:\n%s", v)
	}
}

func TestPreviewRawKeepsTheSearch(t *testing.T) {
	h := openRow(t, sampleFake(), rowAgents)
	h.keys(strings.Split("/anyone", "")...)
	h.keys("enter")
	p := h.top().(*preview)
	if p.pager.Matches() != 1 {
		t.Fatalf("%d matches of anyone in the rendered file, want 1", p.pager.Matches())
	}
	h.run(p.SetRaw(true))
	if p.pager.Query() != "anyone" || p.pager.Matches() != 1 {
		t.Errorf("after raw on the search is %q with %d matches", p.pager.Query(), p.pager.Matches())
	}
}

// Search matches what the rendered file shows, not its markup.
func TestPreviewSearchesTheRenderedText(t *testing.T) {
	f := sampleFake()
	f.addBlob(file("AGENTS.md", 0), "# Title\n\nSome **bold** words.\n")
	h := openRow(t, f, rowAgents)
	p := h.top().(*preview)
	h.keys(strings.Split("/Some bold words", "")...)
	h.keys("enter")
	if p.pager.Matches() != 1 {
		t.Errorf("%d matches of the rendered text, want 1:\n%s", p.pager.Matches(), h.modal())
	}
	h.keys("esc")
	h.keys(strings.Split("/**bold**", "")...)
	h.keys("enter")
	if p.pager.Matches() != 0 {
		t.Errorf("%d matches of the markup in the rendered file, want 0", p.pager.Matches())
	}
}

func TestPreviewRawByConfig(t *testing.T) {
	f := sampleFake()
	h := newHost(loaded(t, f, 40, 12, WithMarkdown(config.MarkdownRaw)))
	h.keys("down", "down", "down", "down", "enter")
	if v := h.modal(); !strings.Contains(v, "# AGENTS.md") {
		t.Errorf("files.markdown=raw doesn't show the source:\n%s", v)
	}
	if raw, ok := sourced(t, h).Raw(); !raw || !ok {
		t.Errorf("Raw() = %v, %v, want true, true", raw, ok)
	}
	// The setting changed for the session applies to the next file.
	cfg := config.Default()
	cfg.Files.Markdown = config.MarkdownRendered
	h.run(func() tea.Msg { return ui.SettingsMsg{Config: cfg} })
	h.keys("q")
	h.keys("enter")
	if v := h.modal(); strings.Contains(v, "# AGENTS.md") {
		t.Errorf("files.markdown=rendered set later doesn't render the next file:\n%s", v)
	}
}

// A file opened on a line or a match shows as its source, which they are
// of.
func TestPreviewOpenedOnALineIsRaw(t *testing.T) {
	f := sampleFake()
	h := newHost(loaded(t, f, 40, 12))
	h.run(func() tea.Msg {
		return ui.OpenFileMsg{Repo: ghTUI, Path: "AGENTS.md", SHA: file("AGENTS.md", 0).SHA, Line: 3}
	})
	if raw, _ := sourced(t, h).Raw(); !raw {
		t.Errorf("a file opened on a line is rendered:\n%s", h.modal())
	}
	h.run(func() tea.Msg { return ui.OpenFileMsg{Repo: ghTUI, Path: "AGENTS.md", SHA: file("AGENTS.md", 0).SHA} })
	if raw, _ := sourced(t, h).Raw(); raw {
		t.Errorf("a file opened at its top shows as its source:\n%s", h.modal())
	}
}

func TestPreviewRawOnlyForMarkdown(t *testing.T) {
	for _, row := range []int{rowGoMod, rowLink, rowGitignore, rowReadme} {
		h := openRow(t, sampleFake(), row)
		s := sourced(t, h)
		if _, ok := s.Raw(); ok {
			t.Errorf("%s: Raw() says it renders", h.top().Title())
		}
		if cmd := s.SetRaw(true); cmd != nil {
			t.Errorf("%s: raw on did something", h.top().Title())
		}
	}
}

func TestViewPreviewRaw(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12, WithMarkdown(config.MarkdownRaw)))
	h.keys("down", "down", "down", "down", "enter")
	v := h.top().View()
	assertFits(t, v, h.width, h.height)
	golden.RequireEqual(t, v)
}

func TestFinderPreviewRendersMarkdown(t *testing.T) {
	for mode, source := range map[string]bool{config.MarkdownRendered: false, config.MarkdownRaw: true} {
		h := newHost(loaded(t, sampleFake(), 40, 12, fast, WithMarkdown(mode)))
		h.width, h.height = 120, 16
		f := findIn(t, h)
		h.keys(strings.Split("agents", "")...)
		v := ansi.Strip(f.View())
		if !strings.Contains(v, "Guidance for anyone.") {
			t.Fatalf("%s: the finder doesn't preview AGENTS.md:\n%s", mode, v)
		}
		if got := strings.Contains(v, "# AGENTS.md"); got != source {
			t.Errorf("%s: the finder shows the source: %v, want %v:\n%s", mode, got, source, v)
		}
	}
}

func TestRepoPath(t *testing.T) {
	tests := []struct {
		file, addr string
		want       string
		rel        bool
	}{
		{"README.md", "logo.png", "logo.png", true},
		{"docs/README.md", "img/logo.png", "docs/img/logo.png", true},
		{"docs/README.md", "./img/logo.png?raw=true#top", "docs/img/logo.png", true},
		{"docs/README.md", "../assets/a.png", "assets/a.png", true},
		{"docs/README.md", "/assets/a.png", "assets/a.png", true},
		{"docs/README.md", "img/my%20logo.png", "docs/img/my logo.png", true},
		{"docs/README.md", "../../outside.png", "", true},
		{"README.md", "#section", "", true},
		{"README.md", "", "", true},
		{"README.md", "https://github.com/o/r/raw/main/a.png", "", false},
		{"README.md", "//example.com/a.png", "", false},
		{"README.md", "data:image/png;base64,AAAA", "", false},
	}
	for _, tt := range tests {
		got, rel := repoPath(tt.file, tt.addr)
		if got != tt.want || rel != tt.rel {
			t.Errorf("repoPath(%q, %q) = %q, %v, want %q, %v", tt.file, tt.addr, got, rel, tt.want, tt.rel)
		}
	}
}

// readmeSource holds images of the repository, by relative and rooted
// addresses, one that names no file, one outside the repository, and one
// on the web, each alone on its line, as pictures are drawn.
const readmeSource = "# Docs\n\n![logo](img/logo.png)\n\n![root](/assets/a.png)\n\n![gone](missing.png)\n\n![out](../../x.png)\n\n![web](https://user-images.githubusercontent.com/1/x.png)\n\nThe end.\n"

// markdownPreview returns the preview of docs/README.md of gh-tui at ref,
// whose content is readmeSource, drawn with images, sized 60 by 40, once
// its content arrived and the files it names were looked up.
func markdownPreview(t *testing.T, images *ui.Images, ref string) (*preview, *fake) {
	t.Helper()
	f := newFake()
	readme := file("README.md", int64(len(readmeSource)))
	f.addTree(ghTUI, ref, dir("docs", "docs-sha"), dir("assets", "assets-sha"))
	f.addTree(ghTUI, "docs-sha", readme, dir("img", "img-sha"))
	f.addTree(ghTUI, "img-sha", file("logo.png", 100))
	f.addTree(ghTUI, "assets-sha", file("a.png", 200))
	f.addBlob(readme, readmeSource)
	readme.Path = "docs/README.md"
	p := newPreview(t.Context(), f, "", ghTUI, ref, readme, key.NewBinding(key.WithKeys("o")), ui.Voice{}, "", ui.NewIcons(""), images, false)
	p.SetTheme(testTheme())
	p.SetSize(60, 40)
	b, err := f.Blob(t.Context(), filesvc.BlobQuery{Repo: ghTUI, SHA: readme.SHA})
	if err != nil {
		t.Fatal(err)
	}
	feed(p, p.Update(blobMsg{id: p.pager.ID(), blob: b}))
	return p, f
}

// feed runs cmd and hands the messages it sends back to p, as the app
// does.
func feed(p *preview, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			feed(p, c)
		}
	case imageEntryMsg, blobMsg:
		feed(p, p.Update(msg))
	}
}

// Images that stand alone draw as pictures: those of the repository read
// as their blobs at the ref browsed, so a private repository's need no
// signed address, and those on the web as comments fetch them. An image
// that names no file, or lies outside the repository, stays text.
func TestPreviewMarkdownPictures(t *testing.T) {
	src := &uitest.ImageHost{}
	images := uitest.Avatars(src, true)
	images.SetMaxRows(4)
	p, f := markdownPreview(t, images, "v1.0")
	for _, q := range f.reads {
		if q.Ref != "v1.0" && !strings.HasSuffix(q.Ref, "-sha") {
			t.Errorf("read the tree at %q, want the ref browsed", q.Ref)
		}
	}
	if _, changed := uitest.LoadAvatars(t, images); !changed {
		t.Fatal("no image arrived")
	}
	feed(p, p.Update(ui.ImagesMsg{}))
	want := []string{"blob eggzec/gh-tui b-a.png", "blob eggzec/gh-tui b-logo.png", "https://user-images.githubusercontent.com/1/x.png"}
	got := slices.Sorted(slices.Values(src.Asked()))
	if !slices.Equal(got, want) {
		t.Errorf("fetched %q, want %q", got, want)
	}
	v := p.View()
	assertFits(t, v, 60, 40)
	if n := uitest.Placeholders(t, v); n == 0 {
		t.Errorf("no cell shows an image:\n%s", ansi.Strip(v))
	}
	text := ansi.Strip(v)
	for _, alt := range []string{"gone", "missing.png", "out", "The end."} {
		if !strings.Contains(text, alt) {
			t.Errorf("the preview lacks %q:\n%s", alt, text)
		}
	}
	for _, alt := range []string{"img/logo.png", "/assets/a.png", "user-images"} {
		if strings.Contains(text, alt) {
			t.Errorf("%q shows as text, not as its picture:\n%s", alt, text)
		}
	}
	// Raw on shows the source, with no pictures.
	feed(p, p.SetRaw(true))
	if v := p.View(); uitest.Placeholders(t, v) != 0 || !strings.Contains(ansi.Strip(v), "![logo](img/logo.png)") {
		t.Errorf("raw on still draws pictures:\n%s", ansi.Strip(v))
	}
}

// Where images aren't drawn, the markdown still renders, with each image
// as its text, and nothing is fetched or looked up.
func TestPreviewMarkdownPicturesOff(t *testing.T) {
	src := &uitest.ImageHost{}
	images := uitest.Avatars(src, false)
	p, f := markdownPreview(t, images, "")
	feed(p, p.Update(ui.ImagesMsg{}))
	if raw, changed := uitest.LoadAvatars(t, images); raw != "" || changed || len(src.Asked()) != 0 {
		t.Errorf("sent %q, fetched %q", raw, src.Asked())
	}
	if len(f.reads) != 0 {
		t.Errorf("looked up the image files: %+v", f.reads)
	}
	text := ansi.Strip(p.View())
	if strings.Contains(text, "# Docs") || !strings.Contains(text, "img/logo.png") {
		t.Errorf("the preview isn't rendered with its images as text:\n%s", text)
	}
}

// A look-up of an image file that fails while GitHub can't be reached
// isn't sent again as images arrive and the file renders again, only
// once GitHub answers again.
func TestPreviewMarkdownLookUpFails(t *testing.T) {
	src := &uitest.ImageHost{}
	images := uitest.Avatars(src, true)
	images.SetMaxRows(4)
	f := newFake()
	readme := file("README.md", int64(len(readmeSource)))
	f.addTree(ghTUI, "", dir("docs", "docs-sha"), dir("assets", "assets-sha"))
	f.addTree(ghTUI, "docs-sha", readme, dir("img", "img-sha"))
	f.addTree(ghTUI, "img-sha", file("logo.png", 100))
	f.addTree(ghTUI, "assets-sha", file("a.png", 200))
	f.errs[treeKey(ghTUI, "img-sha")] = errors.New("502 Bad Gateway")
	f.addBlob(readme, readmeSource)
	readme.Path = "docs/README.md"
	p := newPreview(t.Context(), f, "", ghTUI, "", readme, key.NewBinding(key.WithKeys("o")), ui.Voice{}, "", ui.NewIcons(""), images, false)
	p.SetTheme(testTheme())
	p.SetSize(60, 40)
	b, _ := f.Blob(t.Context(), filesvc.BlobQuery{Repo: ghTUI, SHA: readme.SHA})
	feed(p, p.Update(blobMsg{id: p.pager.ID(), blob: b}))
	imgReads := func() int {
		n := 0
		for _, q := range f.reads {
			if q.Ref == "img-sha" {
				n++
			}
		}
		return n
	}
	if imgReads() != 1 {
		t.Fatalf("%d reads of the failing directory, want 1", imgReads())
	}
	for range 5 {
		uitest.LoadAvatars(t, images)
		feed(p, p.Update(ui.ImagesMsg{}))
	}
	if imgReads() != 1 {
		t.Errorf("%d reads of the failing directory after the images arrived, want 1", imgReads())
	}
	delete(f.errs, treeKey(ghTUI, "img-sha"))
	feed(p, p.Update(ui.OnlineMsg{}))
	if imgReads() != 2 {
		t.Errorf("%d reads of the directory once GitHub answers, want 2", imgReads())
	}
	uitest.LoadAvatars(t, images)
	if !slices.Contains(src.Asked(), "blob eggzec/gh-tui b-logo.png") {
		t.Errorf("the image wasn't fetched once found: %q", src.Asked())
	}
}

// A file of many images looks up and asks for maxBusy of them at once,
// and the rest as those arrive.
func TestPreviewMarkdownBusy(t *testing.T) {
	const n = 10
	var md strings.Builder
	f := newFake()
	entries := make([]core.TreeEntry, 0, n)
	for i := range n {
		name := fmt.Sprintf("i%d.png", i)
		fmt.Fprintf(&md, "![%d](img/%s)\n\n", i, name)
		entries = append(entries, file(name, 10))
	}
	readme := file("README.md", int64(md.Len()))
	f.addTree(ghTUI, "", readme, dir("img", "img-sha"))
	f.addTree(ghTUI, "img-sha", entries...)
	f.addBlob(readme, md.String())
	src := &uitest.ImageHost{}
	images := uitest.Avatars(src, true)
	images.SetMaxRows(2)
	p := newPreview(t.Context(), f, "", ghTUI, "", readme, key.NewBinding(key.WithKeys("o")), ui.Voice{}, "", ui.NewIcons(""), images, false)
	p.SetTheme(testTheme())
	p.SetSize(60, 40)
	b, _ := f.Blob(t.Context(), filesvc.BlobQuery{Repo: ghTUI, SHA: readme.SHA})
	cmd := p.Update(blobMsg{id: p.pager.ID(), blob: b})
	if p.md.looking != maxBusy || len(p.md.pending) != n-maxBusy {
		t.Errorf("%d looked up and %d waiting, want %d and %d", p.md.looking, len(p.md.pending), maxBusy, n-maxBusy)
	}
	feed(p, cmd)
	uitest.LoadAvatars(t, images)
	if got := len(src.Asked()); got != maxBusy {
		t.Errorf("%d images asked for at once, want %d", got, maxBusy)
	}
	for range n {
		feed(p, p.Update(ui.ImagesMsg{}))
		uitest.LoadAvatars(t, images)
	}
	if got := len(src.Asked()); got != n {
		t.Errorf("%d images asked for in the end, want %d", got, n)
	}
}

// The look-ups of a finder hidden behind the preview it opened never
// reach it: once it shows again, the images they were for are looked up
// anew, and load.
func TestFinderMarkdownLookUpsDropped(t *testing.T) {
	const n = maxBusy + 1
	var md strings.Builder
	f := sampleFake()
	entries := make([]core.TreeEntry, 0, n)
	for i := range n {
		name := fmt.Sprintf("i%d.png", i)
		fmt.Fprintf(&md, "![%d](img/%s)\n\n", i, name)
		entries = append(entries, file(name, 10))
	}
	root := slices.Clone(f.trees[treeKey(ghTUI, "")].Entries)
	f.addTree(ghTUI, "", append(root, dir("img", "img-sha"))...)
	f.addTree(ghTUI, "img-sha", entries...)
	f.addBlob(file("AGENTS.md", 0), md.String())
	src := &uitest.ImageHost{}
	images := uitest.Avatars(src, false)
	images.SetMaxRows(2)
	h := newHost(loaded(t, f, 40, 12, fast, WithImages(images)))
	h.width, h.height = 120, 30
	fm := findIn(t, h)
	h.keys(strings.Split("agents", "")...)
	if !strings.Contains(ansi.Strip(fm.View()), "img/i0.png") {
		t.Fatalf("the finder doesn't preview AGENTS.md:\n%s", ansi.Strip(fm.View()))
	}
	// Images begin to show while the finder is hidden: what it looks up
	// is dropped, as the app sends messages to the open modal alone.
	images.SetGraphics(ui.Graphics{Images: true, Cell: uitest.TestCell})
	if cmd := fm.Update(ui.ImagesMsg{}); cmd == nil {
		t.Fatal("no image file was looked up")
	}
	if fm.md.looking != maxBusy {
		t.Fatalf("%d look-ups on their way, want %d", fm.md.looking, maxBusy)
	}
	h.run(func() tea.Msg { return ui.ReopenedMsg{Modal: fm} })
	for range n {
		uitest.LoadAvatars(t, images)
		h.run(func() tea.Msg { return ui.ImagesMsg{} })
	}
	// An image may be asked for again at another width, as the lines of
	// the images arriving widen the gutter of the line numbers.
	if got := slices.Compact(slices.Sorted(slices.Values(src.Asked()))); len(got) != n {
		t.Errorf("%d images asked for once the finder shows again, want %d: %q", len(got), n, got)
	}
	// A look-up of before that arrives late changes nothing.
	if fm.md.take(imageEntryMsg{v: &fm.md, gen: fm.md.gen - 1, path: "img/i0.png"}) {
		t.Error("a look-up of an older generation was taken")
	}
}

// A resize leaves the markdown as rendered until it rests, and then the
// preview asks for one render at the last width.
func TestPreviewMarkdownWaitsOutAResize(t *testing.T) {
	images := uitest.Avatars(&uitest.ImageHost{}, false)
	p, _ := markdownPreview(t, images, "")
	if p.Settle() != nil {
		t.Fatal("a preview at rest waits")
	}
	p.SetSize(40, 40)
	p.SetSize(30, 40)
	if p.Settle() == nil {
		t.Error("a resized preview of markdown has no rest to wait out")
	}
}
