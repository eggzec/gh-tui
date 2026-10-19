package pulls

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// noteAt is an annotation of a.go that ends on line.
func noteAt(l core.AnnotationLevel, line int, msg string) core.Annotation {
	return core.Annotation{Path: "a.go", StartLine: line, EndLine: line, Level: l, Message: msg}
}

// diffFindingChecks serves, on the last commit, a job of "Actions" with a
// warning and a notice on lines 2 and 3 of a.go, a failure on line 99, which
// the diff does not show, and the App "golangci" with a failure on line 2 and
// one on the whole file. The patch of a.go has the lines keep (1), new (2) and
// more (3), and a removed line between 1 and 2.
func diffFindingChecks() *fakeChecks {
	return &fakeChecks{
		runs: map[string][]core.CommitCheck{
			"ddd4444": {
				{ID: 5, JobID: 5, RunID: 9, Name: "build", App: &core.CheckApp{Name: "Actions"}, Annotations: 3, Notes: []core.Annotation{
					noteAt(core.AnnotationWarning, 2, "shadowed variable\nsecond line"),
					noteAt(core.AnnotationNotice, 3, "consider a name"),
					noteAt(core.AnnotationFailure, 99, "far away"),
				}},
				{ID: 7, Name: "lint", App: &core.CheckApp{Name: "golangci"}, Annotations: 2},
			},
			"bbb2222": {
				{ID: 5, JobID: 5, RunID: 9, Name: "build", Annotations: 1, Notes: []core.Annotation{note("docs/c.md", core.AnnotationNotice)}},
			},
		},
		pages: map[int64][]core.Annotation{
			7: {noteAt(core.AnnotationFailure, 2, "bad call"), {Path: "a.go", Level: core.AnnotationNotice, Message: "whole file"}},
		},
	}
}

// findingDiff opens the diff of a.go over the commits, with the findings of fc
// read first and, with threads, the thread that ends on line 3.
func findingDiff(t *testing.T, fc *fakeChecks, threads bool) (*host, *panesModal, *diffOverlay) {
	t.Helper()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(s *fakeService) {
		s.changed = patched()
		s.rangeFiles = &fakeRange{detail: core.CompareDetail{Files: patched()}}
		if threads {
			s.threadPages = map[string]core.ThreadPage{"": {Threads: sampleThreads()[:1], Total: 1}}
		}
	}, WithChecks(fc))
	if cmd := m.readFindings(); cmd != nil {
		drain(t, h, cmd)
	}
	drain(t, h, m.openDiff("a.go"))
	o, ok := m.top().(*diffOverlay)
	if !ok {
		t.Fatalf("top overlay = %T, want the diff", m.top())
	}
	m.SetSize(100, 40)
	modalScreen(t, h)
	return h, m, o
}

// rowOf is the index of the first line of screen that has sub, or -1.
func rowOf(screen, sub string) int {
	for i, l := range strings.Split(screen, "\n") {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

// findingRows counts the lines of screen that are rows of findings.
func findingRows(screen string) int {
	n := 0
	for l := range strings.SplitSeq(screen, "\n") {
		if strings.Contains(l, "Actions: ") || strings.Contains(l, "golangci: ") {
			n++
		}
	}
	return n
}

// The diff of a file with two annotations on a line, one under a thread, one on
// a line it does not show and one on the whole file, at 80 columns.
func TestDiffFindingsView(t *testing.T) {
	t.Parallel()
	p, _ := config.Default().Palette(true)
	h, m, _ := findingDiff(t, diffFindingChecks(), true)
	m.SetTheme(ui.NewTheme(p, true))
	m.SetSize(80, 24)
	press(t, h, "g")
	golden.RequireEqual(t, h.modals[len(h.modals)-1].View())
}

func TestDiffFindingsUnderTheirLines(t *testing.T) {
	t.Parallel()
	h, _, _ := findingDiff(t, diffFindingChecks(), false)
	screen := modalScreen(t, h)
	// The worst level is first, and the message is its first line.
	order := []string{"keep", "old a.go", "new a.go", "golangci: bad call", "Actions: shadowed variable", "more", "Actions: consider a name"}
	prev := -1
	for _, want := range order {
		i := rowOf(screen, want)
		if i <= prev {
			t.Fatalf("%q is on row %d, want after row %d:\n%s", want, i, prev, screen)
		}
		prev = i
	}
	for _, not := range []string{"second line", "far away", "whole file"} {
		if strings.Contains(screen, not) {
			t.Errorf("screen shows %q, which is no row under a shown line:\n%s", not, screen)
		}
	}
	// A removed line has none under it: the next row is the added line.
	if rowOf(screen, "new a.go") != rowOf(screen, "old a.go")+1 {
		t.Errorf("a row sits between the removed line and the next:\n%s", screen)
	}
	if n := findingRows(screen); n != 3 {
		t.Errorf("%d rows of findings, want 3:\n%s", n, screen)
	}
}

// A row of findings is under the thread of its line.
func TestDiffFindingsUnderTheThread(t *testing.T) {
	t.Parallel()
	h, _, _ := findingDiff(t, diffFindingChecks(), true)
	screen := modalScreen(t, h)
	thread, finding := rowOf(screen, "why this?"), rowOf(screen, "Actions: consider a name")
	if thread < 0 || finding < thread {
		t.Errorf("the thread is on row %d, the finding on %d, want it under the thread:\n%s", thread, finding, screen)
	}
}

// The rows are no stops of the cursor, nor of ) and (.
func TestDiffFindingsAreNoStops(t *testing.T) {
	t.Parallel()
	h, _, o := findingDiff(t, diffFindingChecks(), true)
	press(t, h, "g")
	for range 16 {
		press(t, h, "j")
		if id := blockID(o); strings.HasPrefix(id, findingPrefix) {
			t.Fatalf("j stopped on %s", id)
		}
	}
	for _, k := range []string{")", ")", ")", "(", "(", "(", "k"} {
		press(t, h, k)
		if id := blockID(o); strings.HasPrefix(id, findingPrefix) {
			t.Fatalf("%s stopped on %s", k, id)
		}
	}
	press(t, h, "g")
	for range 3 {
		press(t, h, ")")
		if id := blockID(o); !strings.HasPrefix(id, threadPrefix) {
			t.Errorf(") stopped on %q, want the thread", id)
		}
	}
}

// The rows follow the range, and a diff opened over an older commit has none.
func TestDiffFindingsFollowTheRange(t *testing.T) {
	t.Parallel()
	h, m, o := findingDiff(t, diffFindingChecks(), false)
	if n := findingRows(modalScreen(t, h)); n != 3 {
		t.Fatalf("%d rows of findings, want 3", n)
	}
	m.files.rng = files.Range{First: 1, Last: 1}
	cmd := m.readFindings()
	// The findings of the last commit go at once, before the new ones arrive.
	if n := findingRows(modalScreen(t, h)); n != 0 {
		t.Errorf("%d rows while the findings of the range are read, want none", n)
	}
	drain(t, h, cmd)
	if n := findingRows(modalScreen(t, h)); n != 0 {
		t.Errorf("%d rows for a commit that has none on a.go, want none", n)
	}
	m.files.rng = files.All(len(m.files.commits.Commits))
	drain(t, h, m.readFindings())
	o.setBlocks()
	if n := findingRows(modalScreen(t, h)); n != 3 {
		t.Errorf("%d rows of the whole range again, want 3", n)
	}
}

// A diff that a thread opened at an older commit has no rows, as the lines of
// the annotations are those of the head. The diff of a thread that is still on
// the pull request keeps them.
func TestDiffFindingsOfAThreadDiff(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		thread   core.Thread
		wantRows int
	}{
		{"outdated", threadOn("t-stale", "stale words", func(t *core.Thread) {
			t.Outdated, t.OriginalLine = true, 3
			madeOn(longOID("e"))(t)
		}), 0},
		{"current", threadOn("t-line", "current words", func(t *core.Thread) { t.Line = 3 }), 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, m, _ := rangePanes(t, sampleCommits(nil), func(s *fakeService) {
				s.changed = patched()
				for n, tweak := range s.tweaks {
					s.tweaks[n] = func(d *core.PullRequestDetail) { tweak(d); d.HeadSHA = "ddd4444" }
				}
				s.timeline.pages = map[string]core.Page[core.TimelineItem]{"": {Items: convTimeline[:1]}}
				s.threadPages = map[string]core.ThreadPage{"": {Threads: []core.Thread{tc.thread}, Total: 1}}
				s.rangeFiles = &fakeRange{detail: core.CompareDetail{Files: patched()}}
			}, WithChecks(diffFindingChecks()))
			m.SetSize(100, 40)
			if cmd := m.readFindings(); cmd != nil {
				drain(t, h, cmd)
			}
			o, _ := enterThread(t, h, m, tc.thread, conv.KindThread)
			if tc.wantRows == 0 && o.at == "" {
				t.Fatal("the diff is not at the commit of the thread")
			}
			if n := findingRows(modalScreen(t, h)); n != tc.wantRows {
				t.Errorf("%d rows of findings, want %d:\n%s", n, tc.wantRows, modalScreen(t, h))
			}
		})
	}
}

// A fresh read of the findings puts the rows of its new annotations in the
// diff that is open.
func TestDiffFindingsFollowAFreshRead(t *testing.T) {
	t.Parallel()
	fc := diffFindingChecks()
	// The lint is still running: it has no annotations yet.
	lint := fc.runs["ddd4444"][1]
	lint.Status, lint.Annotations = core.RunInProgress, 0
	fc.runs["ddd4444"] = []core.CommitCheck{fc.runs["ddd4444"][0], lint}
	h, m, _ := findingDiff(t, fc, false)
	if screen := modalScreen(t, h); strings.Contains(screen, "bad call") || findingRows(screen) != 2 {
		t.Fatalf("the diff shows the lint before it finished:\n%s", screen)
	}
	fc.mu.Lock()
	fc.runs["ddd4444"] = diffFindingChecks().runs["ddd4444"]
	fc.mu.Unlock()
	drain(t, h, m.Update(syncOf(m, "ddd4444")))
	if screen := modalScreen(t, h); !strings.Contains(screen, "golangci: bad call") || findingRows(screen) != 3 {
		t.Errorf("the diff lacks the rows of the fresh read:\n%s", screen)
	}
}

// While the read is pending or after it failed, the diff has no rows and no toast.
func TestDiffFindingsPendingAndFailed(t *testing.T) {
	t.Parallel()
	fc := diffFindingChecks()
	fc.notedErr = errors.New("offline")
	h, m, _ := rangePanes(t, sampleCommits(nil), func(s *fakeService) {
		s.changed = patched()
		s.rangeFiles = &fakeRange{detail: core.CompareDetail{Files: patched()}}
	}, WithChecks(fc))
	cmd := m.readFindings()
	drain(t, h, m.openDiff("a.go"))
	m.SetSize(100, 40)
	if n := findingRows(modalScreen(t, h)); n != 0 {
		t.Errorf("%d rows while the read is pending, want none", n)
	}
	drain(t, h, cmd)
	screen := modalScreen(t, h)
	if n := findingRows(screen); n != 0 {
		t.Errorf("%d rows after the read failed, want none:\n%s", n, screen)
	}
	if strings.Contains(screen, "offline") {
		t.Errorf("a failed read of the findings shows an error:\n%s", screen)
	}
}
