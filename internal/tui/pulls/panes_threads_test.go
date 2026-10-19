package pulls

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// threadOn is a thread of a.go by alice, with body, three hours old.
func threadOn(id, body string, set func(*core.Thread)) core.Thread {
	t := core.Thread{
		ID: id, Path: "a.go", Side: core.SideRight, StartSide: core.SideRight,
		Comments: []core.ThreadComment{{
			ID: id + "-c", Author: core.User{Login: "alice"}, At: time.Now().Add(-3 * time.Hour), Body: body,
		}},
	}
	set(&t)
	return t
}

// sampleThreads are a thread on two lines, a resolved thread on the deleted
// line, one on the whole file, an outdated one, and one on another file.
func sampleThreads() []core.Thread {
	other := threadOn("t-other", "elsewhere", func(t *core.Thread) { t.Line = 1 })
	other.Path = "other.go"
	return []core.Thread{
		threadOn("t-line", "why this?", func(t *core.Thread) { t.Line, t.StartLine = 3, 2 }),
		threadOn("t-resolved", "fixed it", func(t *core.Thread) {
			t.Line, t.Side, t.StartSide, t.Resolved = 2, core.SideLeft, core.SideLeft, true
		}),
		threadOn("t-file", "about the file", func(t *core.Thread) { t.File = true }),
		threadOn("t-stale", "stale words", func(t *core.Thread) { t.Outdated = true }),
		other,
	}
}

// threadPanes opens the diff of a.go over a pull request that has threads, with
// setup to change the service first.
//
// It also returns how many reads of the threads the diff made, as the
// conversation reads them too.
func threadPanes(t *testing.T, setup func(*fakeService)) (*host, *panesModal, *diffOverlay, func() int) {
	t.Helper()
	var svc *fakeService
	h, m, _ := rangePanes(t, sampleCommits(nil), func(s *fakeService) {
		s.changed = patched()
		svc = s
		setup(s)
	})
	svc.mu.Lock()
	before := len(svc.threadReads)
	svc.mu.Unlock()
	reads := func() int {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return len(svc.threadReads) - before
	}
	drain(t, h, m.openDiff("a.go"))
	o, ok := m.top().(*diffOverlay)
	if !ok {
		t.Fatalf("top overlay = %T, want the diff", m.top())
	}
	modalScreen(t, h)
	return h, m, o, reads
}

// readThreads serves the sample threads as one page the diff has to read.
func readThreads(s *fakeService) {
	s.threadPages = map[string]core.ThreadPage{"": {Threads: sampleThreads(), Total: 5}}
}

// draftThenThreads writes a draft on the diff of a.go, which the rows of the
// threads would be in the way of, then hands the diff threads.
func draftThenThreads(t *testing.T, threads []core.Thread) (*host, *panesModal, *diffOverlay) {
	t.Helper()
	h, m, o, _ := threadPanes(t, func(*fakeService) {})
	writeDraft(t, h, m, o, "one")
	o.receiveThreads(diffThreadsMsg{o: o, threads: threads})
	return h, m, o
}

// The diff of a file with a thread on lines, a resolved one, one on the file
// and a draft together, at 80 columns in both themes.
func TestThreadsDiffView(t *testing.T) {
	t.Parallel()
	for name, dark := range map[string]bool{"dark": true, "light": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, _ := config.Default().Palette(dark)
			h, m, _ := draftThenThreads(t, sampleThreads())
			m.SetTheme(ui.NewTheme(p, dark))
			m.SetSize(80, 24)
			press(t, h, "g")
			golden.RequireEqual(t, h.modals[len(h.modals)-1].View())
		})
	}
}

func TestThreadsShowUnderTheirLines(t *testing.T) {
	t.Parallel()
	h, m, _, reads := threadPanes(t, readThreads)
	m.SetSize(100, 40)
	screen := modalScreen(t, h)
	for _, want := range []string{"why this?", "fixed it", "about the file", "resolved", "alice"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	for _, not := range []string{"stale words", "elsewhere"} {
		if strings.Contains(screen, not) {
			t.Errorf("screen shows %q, which is outdated or of another file:\n%s", not, screen)
		}
	}
	if reads() != 1 {
		t.Errorf("%d reads of the threads, want 1", reads())
	}
}

// The lines of a thread belong to the diff of the whole pull request, so the
// diff of one commit shows neither the thread nor its mark.
func TestThreadsHideInTheDiffOfARange(t *testing.T) {
	t.Parallel()
	h, m, o, _ := threadPanes(t, readThreads)
	m.SetSize(100, 40)
	shown := func() bool {
		screen := modalScreen(t, h)
		text, mark := strings.Contains(screen, "why this?"), strings.Contains(screen, "▎")
		if text != mark {
			t.Errorf("the thread is shown: %t, its mark: %t\n%s", text, mark, screen)
		}
		return text
	}
	if !shown() {
		t.Fatal("the diff of every commit does not show the thread")
	}
	m.files.rng = files.Range{First: 1, Last: 1}
	o.setBlocks()
	if shown() {
		t.Error("the diff of one commit shows the thread")
	}
	m.files.rng = files.All(len(m.files.commits.Commits))
	o.setBlocks()
	if !shown() {
		t.Error("the diff of every commit again does not show the thread")
	}
}

func TestThreadsCachedAreNotRead(t *testing.T) {
	t.Parallel()
	h, _, _, reads := threadPanes(t, func(s *fakeService) {
		s.threadCache = map[string]core.ThreadPage{"": {Threads: sampleThreads(), Total: 5}}
	})
	if screen := modalScreen(t, h); !strings.Contains(screen, "why this?") {
		t.Errorf("the cached thread is not shown:\n%s", screen)
	}
	if reads() != 0 {
		t.Errorf("%d reads of the threads, want none", reads())
	}
}

func TestThreadsFailedReadIsNotRetried(t *testing.T) {
	t.Parallel()
	h, _, o, reads := threadPanes(t, func(s *fakeService) { s.threadsErr = errors.New("boom") })
	for _, k := range []string{"j", "k", "w", "w", "g"} {
		press(t, h, k)
	}
	if o.threadState != threadsFailed {
		t.Fatalf("threads state = %d, want failed", o.threadState)
	}
	if screen := modalScreen(t, h); !strings.Contains(screen, "review threads could not be read") {
		t.Errorf("screen has no line about the failed read:\n%s", screen)
	}
	if reads() != 1 {
		t.Errorf("%d reads of the threads, want 1", reads())
	}
}

// blockID is the ID of the block under the cursor.
func blockID(o *diffOverlay) string {
	b, _ := o.d.CursorBlock()
	return b.ID
}

func TestThreadsPreviousAndNextStopOnDraftsAndThreads(t *testing.T) {
	t.Parallel()
	h, _, o := draftThenThreads(t, sampleThreads()[:3])
	press(t, h, "g")
	order := make([]string, 0, 4)
	for range 4 {
		press(t, h, ")")
		id := blockID(o)
		if strings.HasPrefix(id, draftPrefix) {
			id = draftPrefix
		}
		order = append(order, id)
	}
	// The thread on the whole file is on top, then the one on the deleted line,
	// the draft under line 2 of the new side, and the thread that ends on line 3.
	want := []string{threadPrefix + "t-file", threadPrefix + "t-resolved", draftPrefix, threadPrefix + "t-line"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf(") stopped on %v, want %v", order, want)
	}
	press(t, h, "(")
	if got := blockID(o); !strings.HasPrefix(got, draftPrefix) {
		t.Errorf("( went to %q, want the draft", got)
	}
}

func TestThreadsCannotBeEditedFromTheDiff(t *testing.T) {
	t.Parallel()
	h, m, o, _ := threadPanes(t, readThreads)
	press(t, h, "g")
	press(t, h, ")")
	if !strings.HasPrefix(blockID(o), threadPrefix) {
		t.Fatalf(") did not reach a thread: %q", blockID(o))
	}
	press(t, h, "c")
	if m.top() != o {
		t.Errorf("top = %T after c on a thread, want the diff", m.top())
	}
	press(t, h, "D")
	if m.top() != o {
		t.Errorf("top = %T after D on a thread, want the diff", m.top())
	}
	if got := keptDrafts(m); len(got) != 0 {
		t.Errorf("drafts = %+v, want none", got)
	}
	if len(o.threads) != len(sampleThreads()) {
		t.Errorf("%d threads kept, want %d", len(o.threads), len(sampleThreads()))
	}
}
