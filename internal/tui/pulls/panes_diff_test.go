package pulls

import (
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/diffview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// diffPanes opens the diff of path over the panes of a pull request whose
// files carry patches.
func diffPanes(t *testing.T, path string) (*host, *panesModal, *diffOverlay) {
	t.Helper()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = patched() })
	drain(t, h, m.openDiff(path))
	o, ok := m.top().(*diffOverlay)
	if !ok {
		t.Fatalf("top overlay = %T, want the diff", m.top())
	}
	modalScreen(t, h)
	return h, m, o
}

func TestDiffOpensOnEnterOnAFile(t *testing.T) {
	t.Parallel()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = patched() })
	for range 10 {
		if _, ok := m.hoveredFile(); ok {
			break
		}
		press(t, h, "down")
	}
	path, ok := m.hoveredFile()
	if !ok {
		t.Fatal("no file row was reached")
	}
	press(t, h, "enter")
	o, ok := m.top().(*diffOverlay)
	if !ok || o.path != path {
		t.Fatalf("top = %T, want the diff of %s", m.top(), path)
	}
	if got := modalScreen(t, h); !strings.Contains(got, "new "+path) {
		t.Errorf("screen doesn't show the diff of %s:\n%s", path, got)
	}
	if m.focus != paneFiles {
		t.Errorf("focus %v, want the Files pane to keep it", m.focus)
	}
}

func TestDiffOpensFromTheFinder(t *testing.T) {
	t.Parallel()
	h, m := changedPanes(t, "a1b2c3d", patchedSample()...)
	m.focus = paneConversation
	press(t, h, "ctrl+p")
	typeQuery(t, h, "yaml")
	press(t, h, "enter")
	o, ok := m.top().(*diffOverlay)
	if !ok || o.path != "internal/config/default.yaml" {
		t.Fatalf("top = %T, want the diff of the file chosen", m.top())
	}
	if len(m.overlays) != 1 {
		t.Errorf("%d overlays open, want only the diff", len(m.overlays))
	}
}

func patchedSample() []core.CommitFile {
	fs := changedSample()
	for i := range fs {
		fs[i].Patch = "@@ -1 +1 @@\n-old\n+new " + fs[i].Path + "\n"
	}
	return fs
}

func TestDiffEnterOnAFolderOrCommitDoesNotOpen(t *testing.T) {
	t.Parallel()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = patched() })
	for range 12 {
		if _, ok := m.hoveredFile(); !ok {
			press(t, h, "enter")
			if m.top() != nil {
				t.Fatalf("enter on a row that is no file opened %T", m.top())
			}
		}
		press(t, h, "down")
	}
}

func TestDiffNextFileFollowsTheTree(t *testing.T) {
	t.Parallel()
	h, m, o := diffPanes(t, "a.go")
	press(t, h, "J")
	if o.path != "docs/c.md" || m.found != "docs/c.md" {
		t.Errorf("path %q found %q after J, want docs/c.md", o.path, m.found)
	}
	if got := modalScreen(t, h); !strings.Contains(got, "new docs/c.md") {
		t.Errorf("screen doesn't show docs/c.md:\n%s", got)
	}
	press(t, h, "J")
	press(t, h, "J")
	if o.path != "internal/b.go" {
		t.Errorf("path %q after J at the last file, want it unchanged", o.path)
	}
	press(t, h, "K")
	if o.path != "docs/c.md" {
		t.Errorf("path %q after K, want docs/c.md", o.path)
	}
}

func TestDiffWholePatchKeepsTheFile(t *testing.T) {
	t.Parallel()
	h, _, o := diffPanes(t, "docs/c.md")
	press(t, h, "a")
	if o.mode != diffview.WholePatch {
		t.Fatalf("mode = %v after a, want the whole patch", o.mode)
	}
	if got := modalScreen(t, h); !strings.Contains(got, "Whole patch") || !strings.Contains(got, "docs/c.md") {
		t.Errorf("screen is not the whole patch on docs/c.md:\n%s", got)
	}
	press(t, h, "J")
	if f, _ := o.d.CurrentFile(); f.Path != "internal/b.go" {
		t.Errorf("file under the cursor = %q after J in the whole patch, want internal/b.go", f.Path)
	}
	press(t, h, "a")
	if o.mode != diffview.ThisFile || o.path != "internal/b.go" {
		t.Errorf("mode %v path %q after a again, want the file under the cursor", o.mode, o.path)
	}
}

func TestDiffEscClearsSearchThenCloses(t *testing.T) {
	t.Parallel()
	h, m, o := diffPanes(t, "a.go")
	press(t, h, "/")
	if !o.typing() {
		t.Fatal("/ didn't open the search prompt")
	}
	typeQuery(t, h, "keep")
	press(t, h, "enter")
	if o.typing() {
		t.Fatal("enter left the prompt open")
	}
	if _, ok := m.Act(ui.ActDismiss); !ok || m.top() == nil {
		t.Error("dismiss closed the diff, want it to clear the search first")
	}
	if _, ok := m.Act(ui.ActDismiss); !ok || m.top() != nil {
		t.Error("the second dismiss left the diff open")
	}
}

func TestDiffBackspaceClosesAndQuitClosesTheModal(t *testing.T) {
	t.Parallel()
	_, m, _ := diffPanes(t, "a.go")
	m.Act(ui.ActBack)
	if m.top() != nil || m.closed {
		t.Fatalf("back left overlay %v, modal closed %v, want only the diff closed", m.top() != nil, m.closed)
	}
	m.openDiff("a.go")
	m.Act(ui.ActQuit)
	if !m.closed {
		t.Error("quit left the modal open")
	}
}

func TestDiffMaximize(t *testing.T) {
	t.Parallel()
	_, m, _ := diffPanes(t, "a.go")
	normal := m.placed()
	m.Act(ui.ActMaximize)
	if big := m.placed(); big.W <= normal.W || big.H <= normal.H {
		t.Errorf("maximized %+v is not larger than %+v", big, normal)
	}
}

func TestDiffCloseCancelsTheRead(t *testing.T) {
	t.Parallel()
	_, m, o := diffPanes(t, "a.go")
	if o.ctx.Err() != nil {
		t.Fatal("the read ended while the diff is open")
	}
	m.Act(ui.ActBack)
	if o.ctx.Err() == nil {
		t.Error("closing the diff left its read running")
	}
}

func TestDiffSearchTakesTyping(t *testing.T) {
	t.Parallel()
	h, m, o := diffPanes(t, "a.go")
	press(t, h, "/")
	if _, ok := m.Act(ui.ActBack); ok {
		t.Error("back was taken as an intent while the search prompt types")
	}
	if got := slices.Collect(func(yield func(string) bool) {
		for _, l := range o.layers() {
			for _, b := range l.Short {
				if !yield(b.Help().Desc) {
					return
				}
			}
		}
	}); len(got) == 0 {
		t.Error("the search prompt has no keys in the footer")
	}
}
