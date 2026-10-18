package pulls

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// diffFile is the file of the diff at path.
func diffFile(path string) diff.File { return diff.File{Path: path} }

// viewedPanes opens the diff of path over the panes of a pull request with
// three files, over a service that setup has prepared.
func viewedPanes(t *testing.T, path string, setup func(*fakeService)) (*host, *panesModal, *diffOverlay, *fakeService) {
	t.Helper()
	var fake *fakeService
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) {
		fake = svc
		svc.changed = patched()
		if setup != nil {
			setup(svc)
		}
	})
	drain(t, h, m.openDiff(path))
	o, ok := m.top().(*diffOverlay)
	if !ok {
		t.Fatalf("top overlay = %T, want the diff", m.top())
	}
	modalScreen(t, h)
	return h, m, o, fake
}

func TestViewedMarksOnGitHubFoldsAndMovesOn(t *testing.T) {
	t.Parallel()
	h, m, o, svc := viewedPanes(t, "a.go", nil)
	press(t, h, "a")
	press(t, h, "v")
	if got := svc.sets(); !slices.Equal(got, []string{"a.go=true"}) {
		t.Errorf("marks sent = %v, want a.go marked", got)
	}
	if !o.d.Collapsed("a.go") {
		t.Error("the file viewed is not folded")
	}
	if f, _ := o.d.CurrentFile(); f.Path != "docs/c.md" || m.found != "docs/c.md" {
		t.Errorf("v left the diff on %q, tree on %q, want docs/c.md", f.Path, m.found)
	}
	if o.viewingOf("a.go") != viewedFile || o.viewMark(diffFile("a.go")) == "" {
		t.Error("the file viewed shows no mark")
	}
}

func TestViewedInTheDiffOfAFileMovesToTheNextUnviewed(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "a.go", func(svc *fakeService) { svc.holding(core.Viewed, "docs/c.md") })
	press(t, h, "v")
	if o.path != "internal/b.go" {
		t.Errorf("path %q after v, want the next file not viewed, internal/b.go", o.path)
	}
	if got := svc.sets(); !slices.Equal(got, []string{"a.go=true"}) {
		t.Errorf("marks sent = %v, want a.go marked", got)
	}
}

func TestViewedInTheDiffOfAFileLeavesTheLastFileOpen(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "internal/b.go", nil)
	press(t, h, "v")
	if got := svc.sets(); !slices.Equal(got, []string{"internal/b.go=true"}) {
		t.Errorf("marks sent = %v, want internal/b.go marked", got)
	}
	if o.d.Collapsed("internal/b.go") {
		t.Error("the diff of one file folded the file the user asked to see")
	}
	if o.viewingOf("internal/b.go") != viewedFile {
		t.Error("the file viewed shows no mark")
	}
}

func TestViewedInTheDiffOfAFileKeepsItOpenWhenGitHubRefuses(t *testing.T) {
	t.Parallel()
	h, _, o, _ := viewedPanes(t, "a.go", func(svc *fakeService) {
		svc.holding(core.Viewed, "a.go")
		svc.viewedSendErr = errors.New("refused")
	})
	press(t, h, "v")
	if o.viewingOf("a.go") != viewedFile || o.d.Collapsed("a.go") {
		t.Error("a refused unmark folded the file in the diff of one file")
	}
}

func TestViewedShowsWhatGitHubKeeps(t *testing.T) {
	t.Parallel()
	h, _, o, _ := viewedPanes(t, "a.go", func(svc *fakeService) {
		svc.holding(core.Viewed, "a.go")
		svc.holding(core.ViewedChanged, "docs/c.md")
	})
	press(t, h, "a")
	if !o.d.Collapsed("a.go") {
		t.Error("a file viewed as it is now is not folded")
	}
	if o.d.Collapsed("docs/c.md") {
		t.Error("a file that changed since it was viewed is folded")
	}
	if o.viewingOf("docs/c.md") != changedFile || o.viewMark(diffFile("docs/c.md")) == "" {
		t.Error("the changed file shows no mark that it changed since")
	}
}

func TestViewedTakesTheMarkOff(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "a.go", func(svc *fakeService) { svc.holding(core.Viewed, "a.go") })
	press(t, h, "v")
	if got := svc.sets(); !slices.Equal(got, []string{"a.go=false"}) {
		t.Errorf("marks sent = %v, want a.go unmarked", got)
	}
	if o.path != "a.go" || o.viewingOf("a.go") != notViewed {
		t.Errorf("path %q viewing %v, want a.go left open and not viewed", o.path, o.viewingOf("a.go"))
	}
}

func TestViewedThatGitHubRefusesComesBack(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "a.go", func(svc *fakeService) { svc.viewedSendErr = errors.New("nope") })
	press(t, h, "v")
	if o.viewingOf("a.go") != notViewed {
		t.Error("a mark that GitHub refused stays")
	}
	if len(o.vw.sending) != 0 {
		t.Errorf("%v still sending after the answer", o.vw.sending)
	}
	if got := svc.sets(); len(got) != 1 {
		t.Errorf("marks sent = %v, want one", got)
	}
}

func TestViewedUnmarkThatGitHubRefusesComesBack(t *testing.T) {
	t.Parallel()
	h, _, o, _ := viewedPanes(t, "a.go", func(svc *fakeService) {
		svc.holding(core.Viewed, "a.go")
		svc.viewedSendErr = errors.New("refused")
	})
	// The whole patch folds the file that is viewed, so the refusal shows in it.
	press(t, h, "a")
	if !o.d.Collapsed("a.go") {
		t.Fatal("the file viewed is not folded")
	}
	press(t, h, "v")
	if o.viewingOf("a.go") != viewedFile || !o.d.Collapsed("a.go") {
		t.Error("the mark did not come back after GitHub refused to take it off")
	}
}

func TestViewedReadThatLandsWhileAMarkIsInFlightKeepsTheMark(t *testing.T) {
	t.Parallel()
	h, _, o, _ := viewedPanes(t, "a.go", nil)
	// The whole patch folds the file that is viewed.
	press(t, h, "a")
	// The command isn't run yet, so the answer to the mark isn't in.
	cmd := o.toggleViewed("a.go", false)
	// A read answered before the mark was sent says that nothing is marked.
	o.receiveViewed(viewedMsg{o: o, pv: core.PullViewed{Supported: true}})
	if o.viewingOf("a.go") != viewedFile || !o.d.Collapsed("a.go") {
		t.Error("a read that landed in the meantime took the mark off the screen")
	}
	for _, msg := range msgsOf(cmd) {
		if msg, ok := msg.(viewedSetMsg); ok {
			o.viewedSet(msg)
		}
	}
	if o.viewingOf("a.go") != viewedFile || !o.d.Collapsed("a.go") {
		t.Error("the mark is gone once GitHub answered")
	}
}

func TestViewedSecondPressWhileOneIsInFlightSendsNothing(t *testing.T) {
	t.Parallel()
	_, _, o, svc := viewedPanes(t, "a.go", nil)
	// The command isn't run, so the answer to the first isn't in.
	if o.toggleViewed("a.go", false) == nil {
		t.Fatal("the first press did nothing")
	}
	if cmd := o.toggleViewed("a.go", false); cmd != nil {
		t.Error("a second press of the same file while one is in flight did something")
	}
	if got := svc.sets(); len(got) != 1 {
		t.Errorf("marks sent = %v, want one", got)
	}
}

// msgsOf runs cmd and the commands of the batches it answers with, and returns
// the messages they end in.
func msgsOf(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, msgsOf(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// longFirst makes the first file of the diff, a.go, a patch of n added lines,
// which takes several windows to read.
func longFirst(n int) func(*fakeService) {
	return func(svc *fakeService) {
		patch := fmt.Sprintf("@@ -1,1 +1,%d @@\n keep\n%s", n+1, strings.Repeat("+\tx()\n", n))
		for i := range svc.changed {
			if svc.changed[i].Path == "a.go" {
				svc.changed[i].Patch = patch
			}
		}
	}
}

func TestReadOnPagesInTheMiddleOfAFile(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "a.go", longFirst(80))
	before := o.d.Cursor()
	press(t, h, "space")
	if got := o.current(); got != "a.go" {
		t.Errorf("space left the diff at %q, want the same file", got)
	}
	if after := o.d.Cursor(); after <= before {
		t.Errorf("space left the cursor at row %d, want it below row %d", after, before)
	}
	if sets := svc.sets(); len(sets) != 0 {
		t.Errorf("space in the middle of a file marked it: sent %v", sets)
	}
}

func TestReadOnMarksAFileOfSeveralWindows(t *testing.T) {
	t.Parallel()
	// Lengths that put the end of the file at every phase of a page, among them
	// tails shorter than a window.
	for n := 40; n < 56; n++ {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			h, _, o, svc := viewedPanes(t, "a.go", longFirst(n))
			for presses := 1; len(svc.sets()) == 0; presses++ {
				if presses > 30 {
					t.Fatal("space never marked the first file")
				}
				press(t, h, "space")
				if got := o.current(); len(svc.sets()) == 0 && got != "a.go" {
					t.Fatalf("space left the first file at %q before marking it", got)
				}
			}
			if got := o.current(); got != "docs/c.md" {
				t.Errorf("after marking, the diff is at %q, want the next file", got)
			}
			if sets := svc.sets(); !slices.Equal(sets, []string{"a.go=true"}) {
				t.Errorf("marks sent = %v, want the first file", sets)
			}
		})
	}
}

func TestReadOnMarksAtTheEndAndMovesToTheNextUnviewed(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "a.go", nil)
	press(t, h, "space")
	if got := o.current(); got != "docs/c.md" {
		t.Errorf("space left the diff at %q, want the next file", got)
	}
	if sets := svc.sets(); !slices.Equal(sets, []string{"a.go=true"}) {
		t.Errorf("marks sent = %v, want the first file", sets)
	}
	if o.viewingOf("a.go") != viewedFile {
		t.Error("the file read is not viewed")
	}
}

func TestReadOnWrapsToAnUnviewedFile(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "internal/b.go", func(svc *fakeService) {
		svc.holding(core.Viewed, "docs/c.md")
	})
	press(t, h, "space")
	if got := o.current(); got != "a.go" {
		t.Errorf("space left the diff at %q, want a.go, the only one not viewed", got)
	}
	if sets := svc.sets(); !slices.Equal(sets, []string{"internal/b.go=true"}) {
		t.Errorf("marks sent = %v, want the last file", sets)
	}
}

func TestReadOnWithEveryFileViewedGoesToTheFirst(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "internal/b.go", func(svc *fakeService) {
		svc.holding(core.Viewed, "a.go", "docs/c.md", "internal/b.go")
	})
	press(t, h, "space")
	if got := o.current(); got != "a.go" {
		t.Errorf("space left the diff at %q, want the first file", got)
	}
	if sets := svc.sets(); len(sets) != 0 {
		t.Errorf("reading on over viewed files sent %v, want none", sets)
	}
}

func TestReadOnWithEveryFileViewedPagesOnAndWraps(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "a.go", func(svc *fakeService) {
		svc.holding(core.Viewed, "a.go", "docs/c.md", "internal/b.go")
	})
	// The whole patch shows every file folded, a header each.
	press(t, h, "a")
	if got := o.current(); got != "a.go" {
		t.Fatalf("the diff is at %q, want the first file", got)
	}
	moved, wrapped := false, false
	for range 20 {
		press(t, h, "space")
		got := o.current()
		if got != "a.go" {
			moved = true
		}
		if moved && got == "a.go" {
			wrapped = true
			break
		}
	}
	if !moved || !wrapped {
		t.Errorf("space moved on %v and wrapped to the first file %v, want both", moved, wrapped)
	}
	if sets := svc.sets(); len(sets) != 0 {
		t.Errorf("reading on over viewed files sent %v, want none", sets)
	}
}

func TestReadOnOnlyPagesBeforeTheMarksAreRead(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "a.go", nil)
	o.vw.in, o.vw.failed = false, false
	press(t, h, "space")
	if sets := svc.sets(); len(sets) != 0 || o.viewingOf("a.go") != notViewed {
		t.Errorf("space before the marks were read marked a file: sent %v", sets)
	}
	if got := o.current(); got != "a.go" {
		t.Errorf("space left the diff at %q before the marks were read", got)
	}
}

func TestViewedOnAServerWithoutMarksSaysSo(t *testing.T) {
	t.Parallel()
	h, _, _, svc := viewedPanes(t, "a.go", func(svc *fakeService) { svc.viewedOff = true })
	press(t, h, "v")
	if got := svc.sets(); len(got) != 0 {
		t.Errorf("marks sent = %v, want none", got)
	}
}

func TestViewedBeforeTheMarksAreReadSaysSo(t *testing.T) {
	t.Parallel()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = patched() })
	cmd := m.openDiff("a.go")
	o := m.top().(*diffOverlay)
	if o.vw.in {
		t.Fatal("the marks were read before the read ran")
	}
	if toast := o.toggleViewed("a.go", true); toast == nil {
		t.Error("v before the marks were read said nothing")
	}
	drain(t, h, cmd)
}

func TestViewedAfterAFailedReadSaysSoAndMarksNothing(t *testing.T) {
	t.Parallel()
	h, _, o, svc := viewedPanes(t, "a.go", func(svc *fakeService) { svc.viewedReadErr = errors.New("down") })
	if !o.vw.failed || o.vw.in {
		t.Fatalf("failed %v in %v after a failed read", o.vw.failed, o.vw.in)
	}
	press(t, h, "v")
	if got := svc.sets(); len(got) != 0 {
		t.Errorf("marks sent = %v, want none", got)
	}
}
