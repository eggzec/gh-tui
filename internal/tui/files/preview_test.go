package files

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
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
		{"error", rowGitignore, ".gitignore", "✗ Something went wrong"},
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

// sampleSize returns the size of row i in the sample tree.
func sampleSize(i int) int64 {
	return sampleFake().trees[treeKey(ghTUI, "")].Entries[i].Size
}

func TestPreviewNested(t *testing.T) {
	f := sampleFake()
	h := newHost(loaded(t, f, 40, 12))
	h.keys("+", "down", "+", "down", "enter")
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
	p := newPreview(t.Context(), f, "", ghTUI, "", e, key.NewBinding(key.WithKeys("o")), ui.Voice{})
	p.SetSize(40, 4)
	_ = p.load()
	other := newPreview(t.Context(), f, "", ghTUI, "", e, key.NewBinding(key.WithKeys("o")), ui.Voice{})
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
	if short := short(); !has(short, "o") || !has(short, "q") {
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

func TestPreviewFileClosesOverItsSearch(t *testing.T) {
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
	// A search of the user's own is cleared first, as in any pager.
	h = open()
	h.keys("/", "G", "u", "i", "d", "enter", "esc")
	if h.top() == nil {
		t.Fatal("esc closed the preview over the user's search, want it cleared")
	}
	if p := h.top().(*preview); p.pager.Query() != "" {
		t.Errorf("search %q after esc, want it cleared", p.pager.Query())
	}
	h.keys("esc")
	if h.top() != nil {
		t.Error("esc should close the preview once the search is cleared")
	}
}

// stub is a modal that a preview returns to, which records that it was
// reopened.
type stub struct{ reopened bool }

func (*stub) Title() string              { return "stub" }
func (*stub) View() string               { return "" }
func (*stub) SetSize(int, int)           {}
func (*stub) SetTheme(ui.Theme)          {}
func (*stub) KeyLayers() []keyhelp.Layer { return nil }
func (s *stub) Update(msg tea.Msg) tea.Cmd {
	if msg == (ui.ReopenedMsg{Modal: s}) {
		s.reopened = true
	}
	return nil
}

func TestPreviewFileAtACommitOnALine(t *testing.T) {
	f := sampleFake()
	root := f.trees[treeKey(ghTUI, "")]
	f.addTree(ghTUI, "c0ffee", root.Entries...)
	h := newHost(newSection(t, f, 40, 12))
	h.height = 4
	ret := &stub{}
	h.run(func() tea.Msg {
		return ui.OpenFileMsg{Repo: ghTUI, Path: "cmd/gh-tui/main.go", Ref: "c0ffee", Line: 6, Return: ret}
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
	if len(h.got) == 0 || h.got[0] != (ui.OpenModalMsg{Modal: ret}) || p.ctx.Err() == nil {
		t.Errorf("esc sent %v, want the modal it came from reopened, and the preview's reads ended", h.got)
	}
	if !ret.reopened {
		t.Error("the modal the preview came from wasn't told it is open again")
	}
}

func TestPreviewFileAtACommitNotThere(t *testing.T) {
	f := sampleFake()
	f.addTree(ghTUI, "c0ffee", f.trees[treeKey(ghTUI, "")].Entries...)
	for _, name := range []string{"nope.go", "cmd/gh-tui", "go.mod/x"} {
		h := newHost(newSection(t, f, 40, 12))
		h.run(func() tea.Msg { return ui.OpenFileMsg{Repo: ghTUI, Path: name, Ref: "c0ffee"} })
		if got := strings.Join(strings.Fields(h.modal()), " "); !strings.Contains(got, "✗ No such file at this commit.") {
			t.Errorf("%s: preview = %q, want why it isn't shown", name, got)
		}
	}
}
