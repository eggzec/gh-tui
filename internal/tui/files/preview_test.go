package files

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Rows of the top-level files of the sample.
const (
	rowGitignore = 3
	rowAgents    = 4
	rowLink      = 5
	rowGoMod     = 6
	rowReadme    = 7
)

// openRow returns a host whose section has row i of gh-tui selected and
// enter pressed on it.
func openRow(t *testing.T, f *fake, i int) *host {
	t.Helper()
	h := newHost(loaded(t, f, 40, 12))
	h.keys(slices.Repeat([]string{"down"}, i)...)
	h.keys("enter")
	return h
}

func TestPreview(t *testing.T) {
	tests := []struct {
		name  string
		row   int
		title string
		want  string
	}{
		{"text", rowAgents, "AGENTS.md", "Guidance for anyone."},
		{"too large", rowReadme, "README with spaces.md", "Too large to preview · o opens it in the browser"},
		{"binary", rowGoMod, "go.mod", "Binary file, not shown · o opens it in the browser"},
		{"symlink", rowLink, "CLAUDE.md", "Symbolic link → AGENTS.md"},
		{"error", rowGitignore, ".gitignore", errMark + " Something went wrong"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := sampleFake()
			h := openRow(t, f, tt.row)
			m := h.top()
			if m == nil {
				t.Fatal("enter opened no modal")
			}
			if got := m.Title(); got != tt.title {
				t.Errorf("title = %q, want %q", got, tt.title)
			}
			if got := h.modal(); !strings.Contains(got, tt.want) {
				t.Errorf("preview = %q, want %q in it", got, tt.want)
			}
			if len(f.blobReads) != 1 || f.blobReads[0].Size != sampleSize(tt.row) {
				t.Errorf("blob reads = %+v, want one with the size from the tree", f.blobReads)
			}
		})
	}
}

// A message in the preview, such as a link's arrow or the key that opens a
// file in the browser, is drawn again when the icons change, and the text
// of a file isn't.
func TestPreviewSettingsIcons(t *testing.T) {
	ascii := ui.NewIcons(config.IconsASCII)
	c, err := config.Default().Set("ui.icons", config.IconsASCII)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		row  int
		want string
	}{
		{"symlink", rowLink, "Symbolic link " + ascii.Arrow + " AGENTS.md"},
		{"binary", rowGoMod, "Binary file, not shown" + ascii.Separator + ascii.Key("o") + " opens it in the browser"},
		{"too large", rowReadme, "Too large to preview" + ascii.Separator + ascii.Key("o") + " opens it in the browser"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := openRow(t, sampleFake(), tt.row)
			if got := h.modal(); strings.Contains(got, tt.want) {
				t.Fatalf("preview = %q, already has %q", got, tt.want)
			}
			h.run(func() tea.Msg { return ui.SettingsMsg{Config: c} })
			if got := h.modal(); !strings.Contains(got, tt.want) {
				t.Errorf("preview = %q, want %q in it", got, tt.want)
			}
		})
	}
	t.Run("reloading keeps the spinner", func(t *testing.T) {
		t.Parallel()
		f := sampleFake()
		sha := f.trees[treeKey(ghTUI, "")].Entries[rowGoMod].SHA
		f.blobErrs = map[string]error{sha: core.ErrOffline}
		h := openRow(t, f, rowGoMod)
		p := h.top().(*preview)
		if !p.message {
			t.Fatal("the failed load is no message")
		}
		// The read again, whose fetch is not run, so the load is pending.
		_ = p.Update(ui.OnlineMsg{})
		before := ansi.Strip(p.View())
		_ = p.Update(ui.SettingsMsg{Config: c})
		if p.message {
			t.Error("the pending load is a message")
		}
		if got := ansi.Strip(p.View()); got != before {
			t.Errorf("preview = %q, want it as it was while loading: %q", got, before)
		}
	})
	t.Run("text keeps its place", func(t *testing.T) {
		t.Parallel()
		h := openRow(t, sampleFake(), rowAgents)
		p := h.top().(*preview)
		if p.message {
			t.Fatal("the text of a file is a message")
		}
		h.run(func() tea.Msg { return ui.SettingsMsg{Config: c} })
		if p.message {
			t.Error("the text of a file became a message")
		}
	})
}

// sampleSize returns the size of row i in the sample tree.
func sampleSize(i int) int64 {
	return sampleFake().trees[treeKey(ghTUI, "")].Entries[i].Size
}

// Right and l preview a file, as enter does, where on a folder they expand.
func TestPreviewByExpandKey(t *testing.T) {
	for _, k := range []string{"l", "right"} {
		t.Run(k, func(t *testing.T) {
			h := newHost(loaded(t, sampleFake(), 40, 12))
			h.keys(slices.Repeat([]string{"down"}, rowAgents)...)
			h.keys(k)
			m := h.top()
			if m == nil || m.Title() != "AGENTS.md" {
				t.Fatalf("%s on a file opened %v, want the preview of AGENTS.md", k, m)
			}
		})
	}
}

func TestPreviewNested(t *testing.T) {
	f := sampleFake()
	h := newHost(loaded(t, f, 40, 12))
	h.keys("l", "down", "l", "down", "enter")
	if got := h.top().Title(); got != "cmd/gh-tui/main.go" {
		t.Errorf("title = %q, want the path from the root", got)
	}
	if got := h.modal(); !strings.Contains(got, `fmt.Println("hello")`) {
		t.Errorf("preview = %q, want main.go", got)
	}
}

func TestPreviewFromCache(t *testing.T) {
	f := sampleFake()
	h := openRow(t, f, rowAgents)
	h.keys("q")
	h.keys("enter")
	if n := len(f.blobReads); n != 1 {
		t.Errorf("%d blob reads, want the second preview from the cache", n)
	}
	if got := h.modal(); !strings.Contains(got, "Guidance for anyone.") {
		t.Errorf("preview = %q, want the cached content", got)
	}
}

func TestPreviewSubmodule(t *testing.T) {
	f := sampleFake()
	h := openRow(t, f, 2)
	if h.top() != nil || len(f.blobReads) != 0 {
		t.Errorf("a submodule opened %v and read %v, want neither", h.top(), f.blobReads)
	}
	want := ui.NotifyMsg{Level: toast.Info,
		Text: "vendor-lib is a submodule, with its files in another repository. Press o to open it in the browser."}
	if !slices.Equal(h.got, []tea.Msg{want}) {
		t.Errorf("messages = %#v, want %#v", h.got, want)
	}
}

func TestPreviewOpenInBrowser(t *testing.T) {
	h := openRow(t, sampleFake(), rowReadme)
	h.got = nil
	h.keys("o")
	want := ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/blob/HEAD/README%20with%20spaces.md"}
	if !slices.Equal(h.got, []tea.Msg{want}) {
		t.Errorf("o sent %#v, want %#v", h.got, want)
	}

	// While the search input is open, o is typed into it.
	h = openRow(t, sampleFake(), rowAgents)
	h.got = nil
	h.keys("/", "o")
	if len(h.got) != 0 || !strings.Contains(h.modal(), "/o") {
		t.Errorf("o in the search sent %#v, preview %q", h.got, h.modal())
	}
}

func TestPreviewClose(t *testing.T) {
	h := openRow(t, sampleFake(), rowAgents)
	p, _ := h.top().(*preview)
	h.keys("q")
	if h.top() != nil {
		t.Fatal("q didn't close the preview")
	}
	if !slices.Contains(h.got, tea.Msg(ui.CloseModalMsg{Modal: p})) {
		t.Errorf("messages = %#v, want the preview closed", h.got)
	}
	if p.ctx.Err() == nil {
		t.Error("closing didn't cancel the preview's loads")
	}
	// The tree has the keys again.
	h.got = nil
	h.keys("o")
	if want := (ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/blob/HEAD/AGENTS.md"}); !slices.Equal(h.got, []tea.Msg{want}) {
		t.Errorf("o sent %#v, want %#v", h.got, want)
	}
}

func TestPreviewIgnoresOtherResults(t *testing.T) {
	f := sampleFake()
	e := f.trees[treeKey(ghTUI, "")].Entries[rowAgents]
	p := newPreview(t.Context(), f, "", ghTUI, "", e, key.NewBinding(key.WithKeys("o")), config.Default().Keys, ui.Voice{}, "", ui.NewIcons(""), nil, false)
	p.SetSize(40, 4)
	_ = p.load()
	other := newPreview(t.Context(), f, "", ghTUI, "", e, key.NewBinding(key.WithKeys("o")), config.Default().Keys, ui.Voice{}, "", ui.NewIcons(""), nil, false)
	_ = p.Update(blobMsg{id: other.pager.ID(), err: errNoTree})
	if cmd := p.Update(pager.CloseMsg{ID: other.pager.ID()}); cmd != nil {
		t.Error("the close of another pager closed the preview")
	}
	if got := p.View(); !strings.Contains(got, "Loading") {
		t.Errorf("preview = %q, want it still loading", got)
	}
}

func TestPreviewHelp(t *testing.T) {
	h := openRow(t, sampleFake(), rowAgents)
	has := func(bs []key.Binding, k string) bool {
		return slices.ContainsFunc(bs, func(b key.Binding) bool { return b.Help().Key == k })
	}
	short := func() []key.Binding { return ui.Hints{Layers: h.top().KeyLayers()}.ShortHelp() }
	if short := short(); !has(short, "o") || !has(short, "q/esc") {
		t.Errorf("help lists %v, want the pager keys and o", short)
	}
	h.keys("/")
	if has(short(), "o") {
		t.Error("help lists o while the search input takes it")
	}
}

func TestPreviewFileOfAnotherSection(t *testing.T) {
	// The file is found by a search, before any repository is selected.
	h := newHost(newSection(t, sampleFake(), 40, 12))
	h.run(func() tea.Msg { return ui.OpenFileMsg{Repo: ghTUI, Path: "AGENTS.md", SHA: file("AGENTS.md", 0).SHA} })
	m := h.top()
	if m == nil || m.Title() != "AGENTS.md" {
		t.Fatalf("OpenFileMsg opened %v, want the preview of AGENTS.md", m)
	}
	if got := h.modal(); !strings.Contains(got, "Guidance for anyone.") {
		t.Errorf("preview = %q, want the file", got)
	}
	h.press(press("o"))
	if !slices.Contains(h.got, tea.Msg(ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/blob/HEAD/AGENTS.md"})) {
		t.Errorf("o sent %v, want the file on GitHub", h.got)
	}
}

func TestPreviewFileFinds(t *testing.T) {
	for _, cached := range []bool{true, false} {
		fake := sampleFake()
		s := newSection(t, fake, 40, 12)
		h := newHost(s)
		sha := file("AGENTS.md", 0).SHA
		fake.cachedBlobs[sha] = cached
		h.run(func() tea.Msg { return ui.OpenFileMsg{Repo: ghTUI, Path: "AGENTS.md", SHA: sha, Find: "anyone"} })
		p, ok := h.top().(*preview)
		if !ok {
			t.Fatalf("cached %v: OpenFileMsg opened %v, want a preview", cached, h.top())
		}
		if p.pager.Query() != "anyone" || p.pager.Matches() != 1 {
			t.Errorf("cached %v: search %q with %d matches, want 1 of %q", cached, p.pager.Query(), p.pager.Matches(), "anyone")
		}
		if s.tree != nil {
			t.Errorf("cached %v: the preview of a found file changed the tree to %v", cached, s.repo)
		}
	}
}

func TestPreviewFileEscClosesAtOnce(t *testing.T) {
	sha := file("AGENTS.md", 0).SHA
	open := func() *host {
		h := newHost(newSection(t, sampleFake(), 40, 12))
		h.run(func() tea.Msg { return ui.OpenFileMsg{Repo: ghTUI, Path: "AGENTS.md", SHA: sha, Find: "anyone"} })
		return h
	}
	h := open()
	h.keys("esc")
	if h.top() != nil {
		t.Error("esc should close a preview over the search it opened with")
	}
	// So does one of the user's own.
	h = open()
	h.keys("/", "G", "u", "i", "d", "enter", "esc")
	if h.top() != nil {
		t.Error("esc should close the preview at once over the user's search")
	}
}

func TestPreviewFileAtACommitOnALine(t *testing.T) {
	f := sampleFake()
	root := f.trees[treeKey(ghTUI, "")]
	f.addTree(ghTUI, "c0ffee", root.Entries...)
	h := newHost(newSection(t, f, 40, 12))
	h.height = 4
	h.run(func() tea.Msg {
		return ui.OpenFileMsg{Repo: ghTUI, Path: "cmd/gh-tui/main.go", Ref: "c0ffee", Line: 6}
	})
	p, ok := h.top().(*preview)
	if !ok {
		t.Fatalf("OpenFileMsg opened %v, want a preview", h.top())
	}
	if got := h.modal(); !strings.Contains(got, `fmt.Println("hello")`) || strings.Contains(got, "package main") {
		t.Errorf("preview = %q, want it on line 6", got)
	}
	refs := make([]string, 0, len(f.reads))
	for _, q := range f.reads {
		refs = append(refs, q.Ref)
	}
	if !slices.Equal(refs, []string{"c0ffee", cmdSHA, ghTUISHA}) {
		t.Errorf("read trees %v, want the commit and each directory on the way", refs)
	}
	h.press(press("o"))
	if !slices.Contains(h.got, tea.Msg(ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/blob/c0ffee/cmd/gh-tui/main.go"})) {
		t.Errorf("o sent %v, want the file at the commit", h.got)
	}
	h.got = nil
	h.keys("esc")
	if len(h.got) == 0 || h.got[0] != (ui.CloseModalMsg{Modal: p}) || p.ctx.Err() == nil {
		t.Errorf("esc sent %v, want the preview closed, and its reads ended", h.got)
	}
}

func TestPreviewFileAtACommitNotThere(t *testing.T) {
	f := sampleFake()
	f.addTree(ghTUI, "c0ffee", f.trees[treeKey(ghTUI, "")].Entries...)
	for _, name := range []string{"nope.go", "cmd/gh-tui", "go.mod/x"} {
		h := newHost(newSection(t, f, 40, 12))
		h.run(func() tea.Msg { return ui.OpenFileMsg{Repo: ghTUI, Path: name, Ref: "c0ffee"} })
		if got := strings.Join(strings.Fields(h.modal()), " "); !strings.Contains(got, errMark+" No such file at this commit.") {
			t.Errorf("%s: preview = %q, want why it isn't shown", name, got)
		}
	}
}

// TestPreviewKeysFollowTheConfig checks that the preview pages with the
// keys of its context: a rebound key pages, and the one it replaced
// doesn't.
func TestPreviewKeysFollowTheConfig(t *testing.T) {
	keys := config.Default().Keys
	keys.Set("preview.page_down", []string{"x"})
	e := sampleFake().trees[treeKey(ghTUI, "")].Entries[rowAgents]
	p := newPreview(t.Context(), sampleFake(), "", ghTUI, "", e, key.NewBinding(key.WithKeys("o")), keys, ui.Voice{}, "", ui.NewIcons(""), nil, false)
	p.SetSize(40, 6)
	_ = p.pager.SetContent("long.txt", strings.Repeat("line\n", 100))
	status := func() string {
		for l := range strings.SplitSeq(ansi.Strip(p.View()), "\n") {
			if _, rest, ok := strings.Cut(l, "line "); ok && strings.Contains(rest, "/") {
				return strings.Fields(rest)[0]
			}
		}
		return ""
	}
	if got := status(); got != "1/100" {
		t.Fatalf("status = %q, want the top", got)
	}
	_ = p.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if got := status(); got != "1/100" {
		t.Errorf("space moved to %s, though page down is x", got)
	}
	_ = p.Update(press("x"))
	if got := status(); got == "1/100" {
		t.Error("x didn't page down")
	}
}
