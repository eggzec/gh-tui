package files

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
)

// The files of the sample, which the finder lists: every file of the
// listing, but no directories or submodules.
var sampleFiles = []string{".gitignore", "AGENTS.md", "CLAUDE.md", "cmd/gh-tui/main.go", "go.mod", "README with spaces.md"}

// fast reads the file under the cursor of the finder and of the tree after
// a millisecond, so that tests don't wait.
var fast = prefetching(func(p *config.PrefetchLayers) {
	p.Files.Rest, p.Finder.Rest = new(time.Millisecond), new(time.Millisecond)
	p.Files.Preview.Window = config.Span{Before: new(0), After: new(0)}
})

// findIn opens the finder of the host's section and returns it.
func findIn(t *testing.T, h *host) *finderModal {
	t.Helper()
	mod, cmd := h.s.FindFile()
	if mod == nil {
		t.Fatal("FindFile opened nothing")
	}
	h.run(func() tea.Msg { return ui.OpenModalMsg{Modal: mod} })
	h.run(cmd)
	f, ok := mod.(*finderModal)
	if !ok {
		t.Fatalf("FindFile opened %T", mod)
	}
	return f
}

// listed returns the paths the finder lists, in order.
func listed(f *finderModal) []string {
	var out []string
	for l := range strings.SplitSeq(ansi.Strip(f.find.View()), "\n") {
		l = strings.TrimSpace(strings.TrimPrefix(l, "▌"))
		// A Nerd Font glyph marks the type of the file.
		if r, n := utf8.DecodeRuneInString(l); r >= 0xe000 && r <= 0xf8ff {
			l = strings.TrimSpace(l[n:])
		}
		for _, p := range sampleFiles {
			if strings.HasPrefix(l, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func selectedPath(f *finderModal) string {
	it, _ := f.find.Selected()
	return it.Path
}

func TestFindFileLists(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12))
	h.width, h.height = 60, 12
	f := findIn(t, h)
	if got := f.find.Total(); got != len(sampleFiles) {
		t.Errorf("%d files listed, want %d", got, len(sampleFiles))
	}
	if got := f.Title(); got != "Find file · eggzec/gh-tui" {
		t.Errorf("title = %q", got)
	}
	h.keys("m", "a", "i", "n")
	if got := selectedPath(f); got != "cmd/gh-tui/main.go" {
		t.Errorf("main selects %q", got)
	}
}

// TestFindFileLoadsListing opens the finder before the tree has loaded:
// it reads the listing, once, and the tree starts too.
func TestFindFileLoadsListing(t *testing.T) {
	fk := sampleFake()
	s := New(t.Context(), fk, config.Default().Keys, WithRepo(ghTUI))
	s.SetTheme(testTheme())
	s.SetSize(40, 12)
	h := newHost(s)
	f := findIn(t, h)
	if f.find.Total() != len(sampleFiles) {
		t.Errorf("%d files listed", f.find.Total())
	}
	if n := fk.allCount(); n != 1 {
		t.Errorf("the listing was read %d times, want once", n)
	}
	if !s.started || s.tree.Len() == 0 {
		t.Error("the tree didn't start")
	}
}

func TestFindFileTruncated(t *testing.T) {
	fk := sampleFake()
	fk.truncated[strings.ToLower(ghTUI.String())] = true
	h := newHost(loaded(t, fk, 40, 12))
	f := findIn(t, h)
	if v := ansi.Strip(f.View()); !strings.Contains(v, "listing incomplete") {
		t.Errorf("view = %q, want the note that the listing is cut short", v)
	}
}

func TestFindFileOpensPreviewAndReturns(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12))
	f := findIn(t, h)
	h.keys("m", "a", "i", "n", "enter")
	top := h.top()
	if top == nil || top.Title() != "cmd/gh-tui/main.go" {
		t.Fatalf("enter opened %v, want the preview of main.go", top)
	}
	if !strings.Contains(h.modal(), "hello") {
		t.Errorf("preview = %q", h.modal())
	}
	h.keys("esc")
	if h.top() != ui.Modal(f) {
		t.Fatalf("esc from the preview shows %v, want the finder again", h.top())
	}
	if f.find.Query() != "main" || selectedPath(f) != "cmd/gh-tui/main.go" {
		t.Errorf("finder reopened with %q on %q", f.find.Query(), selectedPath(f))
	}
}

func TestFindFileReveals(t *testing.T) {
	s := loaded(t, sampleFake(), 40, 12)
	h := newHost(s)
	f := findIn(t, h)
	h.keys("m", "a", "i", "n", "ctrl+t")
	if slices.Contains(h.modals, ui.Modal(f)) {
		t.Error("the finder stayed open")
	}
	if !slices.Contains(h.got, tea.Msg(ui.ShowMsg{Title: ui.FilesTitle})) {
		t.Errorf("messages %v, want the files shown", h.got)
	}
	if got := s.selected().Path; got != "cmd/gh-tui/main.go" {
		t.Errorf("tree cursor on %q, want main.go", got)
	}
}

// The finder types, so it has no key that opens a file on GitHub: the
// link of the file does, and so does the tree, where ctrl+t reveals it.
func TestFindFileHasNoBrowserKey(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12))
	f := findIn(t, h)
	h.keys("m", "a", "i", "n", "ctrl+o")
	for _, m := range h.got {
		if _, ok := m.(ui.OpenMsg); ok {
			t.Errorf("messages %v, want no page opened", h.got)
		}
	}
	if !slices.Contains(h.modals, ui.Modal(f)) {
		t.Error("the finder closed")
	}
	for _, b := range f.keys.FullHelp()[0] {
		if b.Help().Desc == "open on GitHub" {
			t.Errorf("help lists %q", b.Help().Key)
		}
	}
}

func TestFindFileCloses(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12))
	f := findIn(t, h)
	h.keys("esc")
	if slices.Contains(h.modals, ui.Modal(f)) {
		t.Error("esc left the finder open")
	}
}

// TestFindFileAtBase lists the files at the base set in the history, and
// forgets the finder of the old base.
func TestFindFileAtBase(t *testing.T) {
	const sha = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	fk := sampleFake()
	fk.addTree(ghTUI, sha, file("OLD.md", 10))
	s := loaded(t, fk, 40, 12)
	h := newHost(s)
	head := findIn(t, h)
	h.keys("esc")
	h.run(func() tea.Msg { return ui.BaseMsg{Repo: ghTUI, Ref: sha, Label: "main @ abcdefa"} })
	if head.ctx.Err() == nil {
		t.Error("the finder of the old base still reads")
	}
	f := findIn(t, h)
	if f == head || f.find.Total() != 1 {
		t.Fatalf("finder lists %d files, want those at the base", f.find.Total())
	}
	if got := f.Title(); got != "Find file · eggzec/gh-tui · main @ abcdefa" {
		t.Errorf("title = %q", got)
	}
	if refs := fk.allRefs(); !slices.Contains(refs, sha) {
		t.Errorf("listings read at %v, want the base", refs)
	}
}

// TestFindFileRecent offers the files opened last first, and keeps the
// finder, with its listing, for the next time.
func TestFindFileRecent(t *testing.T) {
	fk := sampleFake()
	h := newHost(loaded(t, fk, 40, 12))
	h.keys(slices.Repeat([]string{"down"}, rowAgents)...)
	h.keys("enter", "q")
	f := findIn(t, h)
	if got := selectedPath(f); got != "AGENTS.md" {
		t.Errorf("finder opens on %q, want the file opened last", got)
	}
	h.keys("m", "a", "i", "n", "enter", "esc", "esc")
	again := findIn(t, h)
	if again != f || again.find.Query() != "" {
		t.Fatalf("finder opened again as %p with %q", again, again.find.Query())
	}
	if got := listed(again)[:2]; !slices.Equal(got, []string{"cmd/gh-tui/main.go", "AGENTS.md"}) {
		t.Errorf("listed %v first, want the recent files, the latest first", got)
	}
	if n := fk.allCount(); n != 1 {
		t.Errorf("the listing was read %d times", n)
	}
}

func TestFindFilePreview(t *testing.T) {
	png := file("logo.png", 100)
	tests := []struct {
		name  string
		query string
		want  string
		reads int
	}{
		{"text", "agents", "Guidance for anyone.", 1},
		{"too large", "readme", "Too large to preview", 1},
		{"binary", "go.mod", "Binary file, not shown", 1},
		{"binary by name", "logo", "Binary file, not shown", 0},
		{"symlink", "claude", "Symbolic link → AGENTS.md", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fk := sampleFake()
			fk.addTree(ghTUI, "", append(slices.Clone(fk.trees[treeKey(ghTUI, "")].Entries), png)...)
			h := newHost(loaded(t, fk, 40, 12, fast))
			h.width, h.height = 120, 16
			f := findIn(t, h)
			if !f.preview {
				t.Fatal("no preview at 120 columns")
			}
			before := len(fk.blobSHAs())
			h.keys(strings.Split(tt.query, "")...)
			if v := ansi.Strip(f.View()); !strings.Contains(v, tt.want) || strings.Contains(v, "in the browser") {
				t.Errorf("view = %q, want %q and no key that opens it in the browser", v, tt.want)
			}
			if got := len(fk.blobSHAs()) - before; got != tt.reads {
				t.Errorf("%d reads, want %d", got, tt.reads)
			}
		})
	}
}

// TestFindFilePreviewIsNotCounted checks that the file under the finder's
// cursor, which isn't a guess, stays out of the prefetch counts.
func TestFindFilePreviewIsNotCounted(t *testing.T) {
	stats := obs.NewStats()
	defer obs.SetDefault(obs.SetDefault(stats))
	h := newHost(loaded(t, sampleFake(), 40, 12, fast))
	h.width, h.height = 120, 16
	f := findIn(t, h)
	h.keys(strings.Split("agents", "")...)
	if v := ansi.Strip(f.View()); !strings.Contains(v, "Guidance for anyone.") {
		t.Fatalf("view = %q, want the preview", v)
	}
	for _, p := range stats.Summary().Prefetch {
		if p.Kind == "file" && (p.Sent != 0 || p.Read != 0 || p.Opened != 0) {
			t.Errorf("file prefetch counts = %+v, want none for the cursor's file", p)
		}
	}
}

// TestFindFileWindowReadCountsAsUsed checks that a file the finder's window
// read ahead counts as used once the cursor moves onto it and it shows.
func TestFindFileWindowReadCountsAsUsed(t *testing.T) {
	stats := obs.NewStats()
	defer obs.SetDefault(obs.SetDefault(stats))
	opt := prefetching(func(p *config.PrefetchLayers) {
		p.Finder.Rest = new(time.Millisecond)
		p.Finder.Preview.Window = config.Span{Before: new(0), After: new(1)}
		p.Files.Preview.Enabled = new(false)
	})
	h := newHost(loaded(t, filesFake(3), 40, 12, opt))
	h.width, h.height = 120, 16
	f := findIn(t, h)
	next, _ := f.find.At(f.find.Index() + 1)
	h.keys("down")
	if v := ansi.Strip(f.View()); !strings.Contains(v, "package f") {
		t.Fatalf("view = %q, want the preview of %s", v, next.Path)
	}
	for _, p := range stats.Summary().Prefetch {
		if p.Kind == "file" && p.Opened != 1 {
			t.Errorf("file prefetch counts = %+v, want the window's read of %s opened once", p, next.Path)
		}
	}
}

// TestFindFileReadsAhead reads the files around the cursor of the finder
// as its prefetch settings say, besides the one its preview shows.
func TestFindFileReadsAhead(t *testing.T) {
	for _, tt := range []struct {
		name    string
		window  config.Span
		maxSize config.Size
		want    bool
	}{
		{"none by default", config.Span{}, 64 * config.KiB, false},
		{"the next", config.Span{Before: new(0), After: new(1)}, 64 * config.KiB, true},
		{"too large", config.Span{Before: new(0), After: new(1)}, 5, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fk := filesFake(3)
			opt := prefetching(func(p *config.PrefetchLayers) {
				p.Finder.Rest = new(time.Millisecond)
				if tt.window.After != nil {
					p.Finder.Preview.Window = tt.window
				}
				p.Finder.Preview.MaxSize = tt.maxSize
				p.Files.Preview.Enabled = new(false)
			})
			h := newHost(loaded(t, fk, 40, 12, opt))
			h.width, h.height = 120, 16
			f := findIn(t, h)
			// Each file has 10 bytes.
			next, ok := f.find.At(f.find.Index() + 1)
			if !ok {
				t.Fatal("no file after the cursor")
			}
			if got := slices.Contains(fk.blobSHAs(), "b-"+next.Path); got != tt.want {
				t.Errorf("%s read ahead: %v, want %v; read %q", next.Path, got, tt.want, fk.blobSHAs())
			}
		})
	}
}

// TestFindFileReadsAheadWhenReopened loses the message that ends the rest of
// the read ahead, as when the preview of a file replaces the finder then, and
// has the finder reopen with no move of the cursor.
func TestFindFileReadsAheadWhenReopened(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fk := filesFake(4)
		opt := prefetching(func(p *config.PrefetchLayers) {
			p.Finder.Rest = new(time.Millisecond)
			p.Finder.Preview.Window = config.Span{Before: new(0), After: new(1)}
			p.Files.Preview.Enabled = new(false)
		})
		h := newHost(loaded(t, fk, 40, 12, opt))
		h.width, h.height = 120, 16
		f := findIn(t, h)
		// The first window is read at once; the rests are of the moves after it.
		h.keys("down")
		third, ok := f.find.At(f.find.Index() + 2)
		if !ok {
			t.Fatal("no second file after the cursor")
		}
		// The move starts the rest, whose end is never heard: the app sends a
		// hidden modal no messages.
		_ = f.Update(press("down"))
		time.Sleep(10 * time.Millisecond)
		if slices.Contains(fk.blobSHAs(), "b-"+third.Path) {
			t.Fatalf("%s read ahead while the modal was hidden", third.Path)
		}
		h.run(func() tea.Msg { return ui.ReopenedMsg{Modal: f} })
		if !slices.Contains(fk.blobSHAs(), "b-"+third.Path) {
			t.Errorf("%s not read ahead after reopening; read %q", third.Path, fk.blobSHAs())
		}
	})
}

// TestFindFilePreviewCached shows a file read before without waiting for
// the cursor to rest.
func TestFindFilePreviewCached(t *testing.T) {
	fk := sampleFake()
	fk.addBlob(file("AGENTS.md", 0), "# AGENTS.md\n\nGuidance for anyone.\n")
	fk.cachedBlobs[file("AGENTS.md", 0).SHA] = true
	h := newHost(loaded(t, fk, 40, 12))
	h.width, h.height = 120, 16
	f := findIn(t, h)
	h.run(f.Update(tea.PasteMsg{Content: "agents"}))
	if v := ansi.Strip(f.View()); !strings.Contains(v, "Guidance for anyone.") {
		t.Errorf("view = %q, want the cached content", v)
	}
	if slices.Contains(fk.blobSHAs(), file("AGENTS.md", 0).SHA) {
		t.Error("a cached file was read again")
	}
}

func TestFindFileDropsStaleReads(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12, fast))
	h.width, h.height = 120, 16
	f := findIn(t, h)
	h.keys("a", "g", "e")
	before := f.View()
	old := finderBlobMsg{f: f, seq: f.seq - 1, entry: file("go.mod", 900), blob: core.Blob{Content: []byte("stale")}}
	if cmd := f.Update(old); cmd != nil || f.View() != before {
		t.Error("a read for an old cursor was shown")
	}
	if cmd := f.Update(finderRestMsg{f: f, seq: f.seq - 1}); cmd != nil {
		t.Error("a rest for an old cursor started a read")
	}
}

func TestFindFilePreviewToggle(t *testing.T) {
	tests := []struct {
		name  string
		width int
		opts  []Option
		want  bool
	}{
		{"wide", 120, nil, true},
		{"narrow", 80, nil, false},
		{"off", 120, []Option{WithFinderPreview(false)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHost(loaded(t, sampleFake(), 40, 12, append(tt.opts, fast)...))
			h.width, h.height = tt.width, 16
			f := findIn(t, h)
			if f.preview != tt.want {
				t.Fatalf("preview %v, want %v", f.preview, tt.want)
			}
			h.keys("tab")
			if f.preview == tt.want {
				t.Fatal("tab didn't toggle the preview")
			}
			f.SetSize(tt.width, 16)
			if f.preview == tt.want {
				t.Error("a resize undid the toggle")
			}
			assertFits(t, f.View(), tt.width, 16)
		})
	}
}

// The preview owes no render of markdown at its new width once tab shows
// it at another width than it rendered at, nor once the finder comes
// back resized, as the app gives a hidden modal its size but no
// messages: it renders again, or waits out the resize.
func TestFindFilePreviewSettles(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12, fast))
	h.width, h.height = 120, 16
	f := findIn(t, h)
	h.keys(strings.Split("agents", "")...)
	if !strings.Contains(ansi.Strip(f.View()), "Guidance for anyone.") {
		t.Fatalf("the finder doesn't preview AGENTS.md:\n%s", ansi.Strip(f.View()))
	}
	h.keys("tab")
	f.SetSize(110, 16)
	h.keys("tab")
	if f.Settle() != nil {
		t.Error("after tab the preview still owes a render at its new width")
	}
	f.SetSize(130, 16)
	h.run(func() tea.Msg { return ui.ReopenedMsg{Modal: f} })
	if f.Settle() != nil {
		t.Error("back in view the preview still owes a render at its new width")
	}
}

// TestFindFileIgnoresOthers checks that the finder ignores the messages of
// another finder.
func TestFindFileIgnoresOthers(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12))
	f := findIn(t, h)
	if cmd := f.Update(finder.ChosenMsg{ID: -1, Item: finder.Item{Path: "AGENTS.md"}}); cmd != nil {
		t.Error("finder took another's choice")
	}
	if cmd := f.Update(finder.CancelMsg{ID: -1}); cmd != nil {
		t.Error("finder took another's cancel")
	}
}

func TestFindFileWithoutRepo(t *testing.T) {
	s := newSection(t, sampleFake(), 40, 12)
	if mod, cmd := s.FindFile(); mod != nil || cmd != nil {
		t.Error("FindFile opened something without a repository")
	}
}

// TestViewFinder renders the finder at the size inside the frame of a
// terminal of 190 by 50 and of 80 by 24.
func TestViewFinder(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		query         string
	}{
		{"190 columns", 148, 38, "main"},
		{"80 columns", 60, 18, "md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHost(loaded(t, sampleFake(), 40, 12, fast))
			h.width, h.height = tt.width, tt.height
			f := findIn(t, h)
			h.keys(strings.Split(tt.query, "")...)
			v := f.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewFinderFitsNarrowWidths(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12, fast))
	f := findIn(t, h)
	h.keys("m", "d")
	for w := 4; w <= 120; w++ {
		f.SetSize(w, 8)
		assertFits(t, f.View(), w, 8)
	}
}

// TestFindFileAfterRefresh lists the files again once the listing changed.
func TestFindFileAfterRefresh(t *testing.T) {
	fk := sampleFake()
	s := loaded(t, fk, 40, 12)
	h := newHost(s)
	f := findIn(t, h)
	h.keys("esc")
	if again := findIn(t, h); again != f {
		t.Fatal("the finder wasn't kept while the listing stayed")
	}
	h.keys("esc")
	fk.addTree(ghTUI, "", append(slices.Clone(fk.trees[treeKey(ghTUI, "")].Entries), file("NEW.md", 10))...)
	h.keys("r")
	f = findIn(t, h)
	if f.find.Total() != len(sampleFiles)+1 {
		t.Errorf("%d files listed after a refresh, want the new one too", f.find.Total())
	}
}

// The preview of a file, on its own and beside the finder, says what went
// wrong the way the user should read it, without the error's chain,
// request or status code, and names the key that opens the file there,
// which the finder has none of.
func TestPreviewErrorWords(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		preview, finder string
	}{
		{"offline", fmt.Errorf("get blob: github: GET /repos/eggzec/gh-tui/git/blobs/b: %w", core.ErrOffline),
			errMark + " Can't reach GitHub", errMark + " Can't reach GitHub"},
		{"forbidden", fmt.Errorf("get blob: github: 403 Forbidden: %w", core.ErrForbidden),
			errMark + " You don't have access to eggzec/gh-tui · o to open on GitHub", errMark + " You don't have access to eggzec/gh-tui"},
		{"internal", errors.New("get blob: github: decode: unexpected EOF"),
			errMark + " Something went wrong. Details are in the log", errMark + " Something went wrong. Details are in the log"},
	}
	voice := WithVoice(ui.NewVoice(config.Default().Keys, "/var/log/gh-tui.log"))
	clean := func(v string) string { return strings.Join(strings.Fields(ansi.Strip(v)), " ") }
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fk := sampleFake()
			fk.blobErrs[file(".gitignore", 0).SHA] = tt.err
			h := newHost(loaded(t, fk, 40, 12, voice))
			h.width, h.height = 120, 16
			h.keys(slices.Repeat([]string{"down"}, rowGitignore)...)
			h.keys("enter")
			if got := clean(h.modal()); !strings.Contains(got, tt.preview) || strings.Contains(got, "github:") || strings.Contains(got, "403") || strings.Contains(got, "/repos") {
				t.Errorf("preview = %q, want %q", got, tt.preview)
			}

			fk = sampleFake()
			fk.blobErrs[file(".gitignore", 0).SHA] = tt.err
			h = newHost(loaded(t, fk, 40, 12, fast, voice))
			h.width, h.height = 160, 16
			f := findIn(t, h)
			h.keys(strings.Split("gitignore", "")...)
			if got := clean(f.View()); !strings.Contains(got, tt.finder) || strings.Contains(got, "github:") || strings.Contains(got, "403") || strings.Contains(got, "open on GitHub") {
				t.Errorf("finder = %q, want %q", got, tt.finder)
			}
		})
	}
}

// A theme draws the finder once, with its icons in the theme's colors.
func TestFinderThemeDrawsOnce(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 60, 14))
	f := findIn(t, h)
	calls := 0
	f.find.SetIcons(func(it finder.Item) string {
		calls++
		return f.icons.item(it)
	})
	calls = 0
	p, err := config.Default().Palette(false)
	if err != nil {
		t.Fatal(err)
	}
	light := ui.NewTheme(p, false)
	f.SetTheme(light)
	if shown := f.find.Total(); calls == 0 || calls > min(shown, 14) {
		t.Errorf("a theme asked for %d icons, want one per row shown", calls)
	}
	ic := ui.NewIcons(config.IconsNerd)
	goIcon := ic.Entry(core.TreeEntry{Name: "main.go", Type: core.EntryBlob}, false)
	if want := light.FileIcon(goIcon).Render(goIcon.Glyph); !strings.Contains(f.find.View(), want) {
		t.Errorf("no light Go icon %q in\n%s", want, f.find.View())
	}
}
