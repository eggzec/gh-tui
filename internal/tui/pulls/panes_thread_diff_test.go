package pulls

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// madeOn sets the commit that a thread was made on.
func madeOn(commit string) func(*core.Thread) {
	return func(t *core.Thread) { t.Comments[0].OriginalCommit = commit }
}

// enterPanes opens the modal over a pull request with threads. The diff of a
// commit has a fourth line on the new side that the diff of the pull request
// lacks.
func enterPanes(t *testing.T, threads []core.Thread) (*host, *panesModal, *fakeService) {
	t.Helper()
	orig := patched()
	for i := range orig {
		if orig[i].Path == "a.go" {
			orig[i].Patch = "@@ -1,2 +1,4 @@\n keep\n-old a.go\n+new a.go\n+more\n+extra\n"
		}
	}
	h, m, svc := rangePanes(t, sampleCommits(nil), func(s *fakeService) {
		s.changed = patched()
		// A detail with a head starts the Conversation, which shows a thread under
		// its review.
		for n, tweak := range s.tweaks {
			s.tweaks[n] = func(d *core.PullRequestDetail) { tweak(d); d.HeadSHA = "ddd4444" }
		}
		s.timeline.pages = map[string]core.Page[core.TimelineItem]{"": {Items: convTimeline[:1]}}
		s.threadPages = map[string]core.ThreadPage{"": {Threads: threads, Total: len(threads)}}
		s.rangeFiles = &fakeRange{detail: core.CompareDetail{Files: orig}}
	})
	m.SetSize(100, 40)
	return h, m, svc
}

// enterThread presses enter on a row of kind that stands for th, and returns
// the diff it opened and the messages that came.
func enterThread(t *testing.T, h *host, m *panesModal, th core.Thread, kind string) (o *diffOverlay, msgs []tea.Msg) {
	t.Helper()
	m.focus = panes.Conversation
	seq, found := m.files.seq, m.found
	cmd := m.Update(conv.EnterMsg{ID: "x", Kind: kind, Thread: &th, Comment: &th.Comments[0]})
	// The reads that the cmd brings sync the viewed marks, which bumps the draw
	// key of the Files pane as well, so the key is checked before they come.
	if m.files.seq != seq || m.found != found {
		t.Errorf("enter revealed a file: seq %d, file %q; want %d, %q", m.files.seq, m.found, seq, found)
	}
	msgs = drain(t, h, cmd)
	o, ok := m.top().(*diffOverlay)
	if !ok {
		t.Fatalf("top = %T, want the diff", m.top())
	}
	return o, msgs
}

// Esc and backspace close the diff that a thread opened, and the Conversation
// keeps its cursor on the thread.
func TestEnterThreadDiffClosesWithEscAndBackspace(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		act  string
	}{
		{"esc", ui.ActDismiss},
		{"backspace", ui.ActBack},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			th := threadOn("t-close", "close words", func(t *core.Thread) { t.Line, t.ReviewID = 3, "R1" })
			h, m, _ := enterPanes(t, []core.Thread{th})
			modalScreen(t, h)
			drain(t, h, m.jumpConv(convWant{id: "R1"}))
			press(t, h, "down")
			id, kind, ok := m.conv.model.Selected()
			if !ok || kind != conv.KindThread {
				t.Fatalf("selected %q of kind %q, %v; want a thread row", id, kind, ok)
			}
			enterThread(t, h, m, th, conv.KindThread)
			m.Act(tc.act)
			if m.top() != nil || m.closed {
				t.Fatalf("overlay open %v, modal closed %v, want only the diff closed", m.top() != nil, m.closed)
			}
			if gotID, gotKind, ok := m.conv.model.Selected(); !ok || gotID != id || gotKind != kind {
				t.Errorf("selected %q of kind %q, %v; want %q of kind %q", gotID, gotKind, ok, id, kind)
			}
		})
	}
}

func TestEnterThreadOpensTheDiffAtItsLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		set    func(*core.Thread)
		want   diff.Pos
		header bool
	}{
		{"line", func(t *core.Thread) { t.Line = 3 }, diff.Pos{Path: "a.go", Side: diff.NewSide, Line: 3}, false},
		{"range", func(t *core.Thread) { t.Line, t.StartLine = 3, 2 }, diff.Pos{Path: "a.go", Side: diff.NewSide, Line: 2}, false},
		{"old side", func(t *core.Thread) { t.Line, t.Side, t.StartSide = 2, core.SideLeft, core.SideLeft }, diff.Pos{Path: "a.go", Side: diff.OldSide, Line: 2}, false},
		{"file", func(t *core.Thread) { t.File = true }, diff.Pos{}, true},
		{"line not in the diff", func(t *core.Thread) { t.Line = 99 }, diff.Pos{}, true},
	} {
		for _, kind := range []string{conv.KindThread, conv.KindReply} {
			t.Run(tc.name+"/"+kind, func(t *testing.T) {
				t.Parallel()
				th := threadOn("t-enter", "enter words", tc.set)
				h, m, svc := enterPanes(t, []core.Thread{th})
				before, found := m.files.rng, m.found
				o, _ := enterThread(t, h, m, th, kind)
				// Checked before the first View, which draws the Files pane and
				// clears the one-shot found.
				if m.files.rng != before || m.found != found {
					t.Errorf("range %+v, file %q; want %+v, %q", m.files.rng, m.found, before, found)
				}
				if o.at != "" || o.path != "a.go" {
					t.Errorf("diff at %q of %q, want the pull request's diff of a.go", o.at, o.path)
				}
				got, onLine := o.d.Position()
				if tc.header {
					if onLine {
						t.Errorf("cursor on the line %+v, want the file's header", got)
					}
				} else if !onLine || got != tc.want {
					t.Errorf("cursor at %+v, %v; want %+v", got, onLine, tc.want)
				}
				if !tc.header && !strings.Contains(modalScreen(t, h), "enter words") {
					t.Errorf("the thread is not shown under its line:\n%s", modalScreen(t, h))
				}
				svc.mu.Lock()
				defer svc.mu.Unlock()
				if r := svc.rangeFiles; len(r.reads) > 0 && r.reads[len(r.reads)-1].Head == longOID("e") {
					t.Errorf("an up to date thread read the diff of a commit: %+v", r.reads)
				}
			})
		}
	}
}

func TestEnterOutdatedThreadOpensTheDiffAtItsCommit(t *testing.T) {
	t.Parallel()
	commit := longOID("e")
	stale := threadOn("t-stale", "stale words", func(t *core.Thread) {
		t.Outdated, t.OriginalLine = true, 4
		madeOn(commit)(t)
	})
	elsewhere := threadOn("t-else", "other commit words", func(t *core.Thread) {
		t.Outdated, t.OriginalLine = true, 4
		madeOn(longOID("f"))(t)
	})
	current := threadOn("t-line", "current words", func(t *core.Thread) { t.Line = 3 })
	h, m, svc := enterPanes(t, []core.Thread{stale, elsewhere, current})
	before, found := m.files.rng, m.found
	o, _ := enterThread(t, h, m, stale, conv.KindThread)
	// Checked before the first View, which draws the Files pane and clears the
	// one-shot found.
	if m.files.rng != before || m.found != found {
		t.Errorf("range %+v, file %q; want %+v, %q", m.files.rng, m.found, before, found)
	}

	svc.mu.Lock()
	reads := slices.Clone(svc.rangeFiles.reads)
	svc.mu.Unlock()
	want := pulls.RangeQuery{Repo: m.repo, Base: "base000", Head: commit}
	if len(reads) == 0 || reads[len(reads)-1] != want {
		t.Errorf("range reads = %+v, want the last to be %+v", reads, want)
	}
	if o.at != commit {
		t.Errorf("diff at %q, want %q", o.at, commit)
	}
	wantPos := diff.Pos{Path: "a.go", Side: diff.NewSide, Line: 4}
	if got, ok := o.d.Position(); !ok || got != wantPos {
		t.Errorf("cursor at %+v, %v; want %+v", got, ok, wantPos)
	}
	screen := modalScreen(t, h)
	if !strings.Contains(screen, "stale words") {
		t.Errorf("the thread is not shown under its line:\n%s", screen)
	}
	for _, not := range []string{"other commit words", "current words"} {
		if strings.Contains(screen, not) {
			t.Errorf("screen shows %q, which is not a thread of this commit:\n%s", not, screen)
		}
	}
	if !strings.Contains(o.title(), commit[:7]) {
		t.Errorf("title %q lacks the commit %s", o.title(), commit[:7])
	}
	if _, marked := o.marks[wantPos]; !marked {
		t.Errorf("the line %+v carries no mark: %v", wantPos, o.marks)
	}
}

// Stepping to another file in a diff that a thread opened leaves the Files pane
// as it is, for the diff of the pull request and for that of a commit.
func TestStepInThreadDiffLeavesTheFilesPane(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		set  func(*core.Thread)
	}{
		{"current", func(t *core.Thread) { t.Line = 3 }},
		{"outdated", func(t *core.Thread) {
			t.Outdated, t.OriginalLine = true, 4
			madeOn(longOID("e"))(t)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			th := threadOn("t-step", "step words", tc.set)
			h, m, _ := enterPanes(t, []core.Thread{th})
			before, found := m.files.rng, m.found
			o, _ := enterThread(t, h, m, th, conv.KindThread)
			seq := m.files.seq
			drain(t, h, o.step(1))
			if o.path == "a.go" {
				t.Fatalf("the diff is still of %q, want another file", o.path)
			}
			if m.files.rng != before || m.found != found || m.files.seq != seq {
				t.Errorf("range %+v, file %q, seq %d; want %+v, %q, %d", m.files.rng, m.found, m.files.seq, before, found, seq)
			}
		})
	}
}

// A thread whose line is 0 and whose original line is set is outdated, even when
// GitHub does not say so.
func TestEnterThreadWithNoLineLeftIsOutdated(t *testing.T) {
	t.Parallel()
	commit := longOID("e")
	th := threadOn("t-gone", "gone words", func(t *core.Thread) {
		t.OriginalLine, t.OriginalStartLine = 4, 3
		madeOn(commit)(t)
	})
	h, m, _ := enterPanes(t, []core.Thread{th})
	o, _ := enterThread(t, h, m, th, conv.KindReply)
	wantPos := diff.Pos{Path: "a.go", Side: diff.NewSide, Line: 3}
	if got, ok := o.d.Position(); o.at != commit || !ok || got != wantPos {
		t.Errorf("diff at %q, cursor %+v, %v; want %q and %+v", o.at, got, ok, commit, wantPos)
	}
}

func TestEnterOutdatedThreadWithoutItsCommitOpensTheCurrentDiff(t *testing.T) {
	t.Parallel()
	th := threadOn("t-stale", "stale words", func(t *core.Thread) { t.Outdated, t.OriginalLine = true, 4 })
	h, m, svc := enterPanes(t, []core.Thread{th})
	found := m.found
	o, msgs := enterThread(t, h, m, th, conv.KindThread)
	if o.at != "" {
		t.Errorf("diff at %q, want the current diff", o.at)
	}
	if m.found != found {
		t.Errorf("file %q, want %q", m.found, found)
	}
	if got, ok := o.d.Position(); ok {
		t.Errorf("cursor on the line %+v, want the file's header", got)
	}
	want := ui.NotifyMsg{Level: toast.Info, Text: "The commit of this thread is no longer available."}
	if !slices.Contains(msgs, tea.Msg(want)) {
		t.Errorf("messages %v lack the toast %v", msgs, want)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, q := range svc.rangeFiles.reads {
		if q.Head == "" {
			t.Errorf("read the range %+v of no commit", q)
		}
	}
}
