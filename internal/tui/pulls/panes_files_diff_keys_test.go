package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/diffview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// focusZoomedDiff moves the tree to a.go and gives the diff beside
// it the focus.
func focusZoomedDiff(t *testing.T, h *host, m *panesModal) *diffOverlay {
	t.Helper()
	const path = "a.go"
	for range 12 {
		if p, ok := m.hoveredFile(); ok && p == path {
			break
		}
		press(t, h, "down")
	}
	if p, _ := m.hoveredFile(); p != path {
		t.Fatalf("the tree is on %q, want %q", p, path)
	}
	press(t, h, "tab")
	pv := &m.files.pv
	if !pv.on || !pv.o.d.Focused() || pv.path != path {
		t.Fatalf("tab left the diff of %q focused=%v", pv.path, pv.on && pv.o.d.Focused())
	}
	return pv.o
}

func TestFilesDiffDraftsAreSharedWithTheModal(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	o := focusZoomedDiff(t, h, m)
	writeDraft(t, h, m, o, "from beside")
	if got := keptDrafts(m); len(got) != 1 || got[0].Body != "from beside" || got[0].Path != "a.go" {
		t.Fatalf("drafts = %+v, want one on a.go", got)
	}
	if got := modalScreen(t, h); !strings.Contains(got, "✎ draft L1–2: from beside") {
		t.Errorf("the diff beside the tree shows no draft row:\n%s", got)
	}
	press(t, h, "tab")
	press(t, h, "enter")
	mo, ok := m.top().(*diffOverlay)
	if !ok || mo.noteCount() != 1 {
		t.Fatalf("top = %T, the modal diff should show the draft", m.top())
	}
	onFirstLine(t, h, mo)
	for _, k := range []string{"V", "j", "c"} {
		press(t, h, k)
	}
	typeText(m, "from modal")
	press(t, h, "ctrl+s")
	// The app turns backspace into the back intent before the modal sees it.
	m.Act(ui.ActBack)
	if m.top() != nil {
		t.Fatalf("backspace left %T open", m.top())
	}
	if o.noteCount() != 2 || m.files.pv.o != o {
		t.Errorf("the diff beside the tree has %d drafts, want 2 of the same diff", o.noteCount())
	}
}

func TestFilesDiffDeletesADraft(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	o := focusZoomedDiff(t, h, m)
	writeDraft(t, h, m, o, "gone soon")
	press(t, h, ")")
	if _, ok := o.draftAt(); !ok {
		t.Fatal(") did not reach the draft")
	}
	press(t, h, "D")
	if _, ok := m.top().(*confirmOverlay); !ok {
		t.Fatalf("D left %T on top, want the question", m.top())
	}
	press(t, h, "y")
	if got := keptDrafts(m); len(got) != 0 || o.noteCount() != 0 {
		t.Errorf("drafts = %+v, shown %d after y, want none", got, o.noteCount())
	}
}

func TestFilesDiffStopsOnDraftsAndThreads(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	o := focusZoomedDiff(t, h, m)
	writeDraft(t, h, m, o, "one")
	m.Update(diffThreadsMsg{o: o, threads: sampleThreads()})
	for name, move := range map[string][2]string{"next": {"g", ")"}, "previous": {"G", "("}} {
		var draft, thread bool
		press(t, h, move[0])
		for range 8 {
			press(t, h, move[1])
			if b, ok := o.d.CursorBlock(); ok {
				draft = draft || isDraft(b)
				thread = thread || isThread(b)
			}
		}
		if !draft || !thread {
			t.Errorf("%s: stopped on a draft %v, on a thread %v", name, draft, thread)
		}
	}
}

func TestFilesDiffTogglesTheFullFile(t *testing.T) {
	t.Parallel()
	blobs := &fakeBlobs{blobs: map[string]core.Blob{
		"sha-a.go": {Content: []byte("keep\nnew a.go\nmore\nafter the hunk\n")},
	}}
	fs := patched()
	for i := range fs {
		fs[i].SHA = "sha-" + fs[i].Path
	}
	h, m, _ := zoomedFiles(t, fs, nil, WithBlobs(blobs))
	o := focusZoomedDiff(t, h, m)
	press(t, h, "x")
	if !o.d.Full("a.go") {
		t.Fatal("x did not show the file whole")
	}
	press(t, h, "x")
	if o.d.Full("a.go") {
		t.Error("x did not go back to the hunks")
	}
}

func TestFilesDiffStepsThroughTheFilesAndTheTreeFollows(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	o := focusZoomedDiff(t, h, m)
	for _, k := range []string{"J", "space"} {
		before := m.files.pv.path
		press(t, h, k)
		modalScreen(t, h)
		pv := &m.files.pv
		if pv.path == before || pv.o.current() != pv.path {
			t.Fatalf("%s left the diff on %q (was %q), shows %q", k, pv.path, before, pv.o.current())
		}
		if p, ok := m.hoveredFile(); !ok || p != pv.path {
			t.Errorf("%s: the tree is on %q, want %q", k, p, pv.path)
		}
		if pv.o != o {
			t.Errorf("%s built a new diff in place of stepping in the one open", k)
		}
	}
	press(t, h, "K")
	modalScreen(t, h)
	if p, _ := m.hoveredFile(); p != m.files.pv.path {
		t.Errorf("K: the tree is on %q, want %q", p, m.files.pv.path)
	}
}

func TestFilesDiffKeepsTheWholePatch(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	o := focusZoomedDiff(t, h, m)
	press(t, h, "a")
	modalScreen(t, h)
	if o.mode != diffview.WholePatch || m.files.pv.o != o {
		t.Fatalf("a left mode %v on the same diff %v", o.mode, m.files.pv.o == o)
	}
	press(t, h, "a")
	modalScreen(t, h)
	if o.mode != diffview.ThisFile {
		t.Error("a did not go back to the file")
	}
}

func TestFilesDiffEscClearsTheRangeBeforeClosing(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	o := focusZoomedDiff(t, h, m)
	onFirstLine(t, h, o)
	press(t, h, "V")
	if _, ok := o.d.Anchor(); !ok {
		t.Fatal("V started no range")
	}
	// The app turns esc into the dismiss intent before the modal sees it.
	m.Act(ui.ActDismiss)
	if _, ok := o.d.Anchor(); ok || m.closed {
		t.Fatalf("esc: range kept %v, modal closed %v", !ok, m.closed)
	}
	m.Act(ui.ActDismiss)
	if !m.closed {
		t.Error("the second esc did not close the modal")
	}
}

func TestFilesDiffIgnoresZ(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	o := focusZoomedDiff(t, h, m)
	at, mode, zoom := o.d.Cursor(), o.mode, m.zoom.Zoomed()
	press(t, h, "Z")
	if o.d.Cursor() != at || o.mode != mode || m.zoom.Zoomed() != zoom || m.closed || m.top() != nil {
		t.Error("Z acted in the diff beside the tree")
	}
	if m.sub[paneFiles-1] != panes.Preview {
		t.Error("Z moved the focus")
	}
}

func TestFilesDiffShowsTheDraftsWhileTheTreeHasTheFocus(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	o := focusZoomedDiff(t, h, m)
	writeDraft(t, h, m, o, "still here")
	press(t, h, "tab")
	if o.d.Focused() {
		t.Fatal("tab left the diff focused")
	}
	if got := modalScreen(t, h); !strings.Contains(got, "✎ draft L1–2: still here") {
		t.Errorf("the unfocused diff shows no draft:\n%s", got)
	}
}

func TestFilesDiffDropsTheDraftsSentWithAReview(t *testing.T) {
	t.Parallel()
	h, m, svc := zoomedFiles(t, patched(), nil)
	o := focusZoomedDiff(t, h, m)
	writeDraft(t, h, m, o, "goes out")
	if o.noteCount() != 1 {
		t.Fatalf("the diff shows %d drafts before the review, want 1", o.noteCount())
	}
	reviewWithComposer(m, "see notes")
	submit(m)
	msg, ok := find[reviewedMsg](m.Update(keyMsg("y")))
	if !ok {
		t.Fatal("yes sent nothing")
	}
	if got := svc.submits(); len(got) != 1 || len(got[0].drafts) != 1 {
		t.Fatalf("submitted %+v, want one review with the draft", got)
	}
	m.Update(msg)
	if m.files.pv.o != o {
		t.Fatal("the review rebuilt the diff, which would hide the stale draft")
	}
	if o.noteCount() != 0 {
		t.Errorf("the diff beside the tree still has %d drafts after they were sent", o.noteCount())
	}
	if got := modalScreen(t, h); strings.Contains(got, "✎ draft") {
		t.Errorf("the diff beside the tree still shows the sent draft:\n%s", got)
	}
}

func TestFilesDiffTitleKeepsTheFileAndThePercent(t *testing.T) {
	t.Parallel()
	h, m, _ := zoomedFiles(t, patched(), nil)
	focusZoomedDiff(t, h, m)
	got := ansi.Strip(modalScreen(t, h))
	if !strings.Contains(got, "a.go (1/3)") || !strings.Contains(got, "hunks only · 0%") {
		t.Errorf("the title of the diff beside the tree lost the file or the percentage:\n%s", got)
	}
	if strings.Contains(got, "hunks only [") {
		t.Errorf("the title of the diff beside the tree has the key hint:\n%s", got)
	}
}
