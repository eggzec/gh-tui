package threads

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

func TestOpen(t *testing.T) {
	const discussions = "https://github.com/charmbracelet/glow/discussions?discussions_q=Themes"
	runs := note(core.SubjectCheckSuite, 9, 0)
	runs.Subject.Title = "ci workflow run failed for feat/search-page branch"
	badRun := note(core.SubjectCheckSuite, 10, 0)
	badRun.Subject.Title = "CI run failed on main"
	discussion := note(core.SubjectDiscussion, 0, 0)
	discussion.Subject.WebURL = discussions
	alert := note("RepositoryVulnerabilityAlert", 11, 0)
	noNumber := note(core.SubjectPullRequest, 0, 0)
	tests := []struct {
		name  string
		n     core.Notification
		want  tea.Msg
		toast string
	}{
		{"issue", note(core.SubjectIssue, 238, 0), ui.OpenIssueMsg{Repo: glow, Number: 238}, ""},
		{"pull request", note(core.SubjectPullRequest, 239, 0), ui.OpenPullMsg{Repo: glow, Number: 239}, ""},
		{"release", note(core.SubjectRelease, 368759772, 0), ui.OpenReleaseMsg{Repo: glow, ID: 368759772, URL: "https://github.com/charmbracelet/glow/368759772"}, ""},
		{"commit", note(core.SubjectCommit, 7, 0), ui.OpenCommitMsg{Repo: glow, SHA: "c0ffee7"}, ""},
		{"run", runs, ui.OpenActionsMsg{Repo: glow, Filter: core.RunFilter{Branch: "feat/search-page", Status: "failure"}}, ""},
		{"discussion", discussion, ui.OpenMsg{URL: discussions}, "Opened in the browser — gh-tui has no discussion view yet"},
		{"run without a branch", badRun, ui.OpenMsg{URL: badRun.Subject.WebURL}, "Opened in the browser — gh-tui has no view of it yet"},
		{"other", alert, ui.OpenMsg{URL: alert.Subject.WebURL}, "Opened in the browser — gh-tui has no view of it yet"},
		{"pull request without a number", noNumber, ui.OpenMsg{URL: noNumber.Subject.WebURL}, "Opened in the browser — gh-tui has no view of it yet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, o := range []*Opener{nil, New(t.Context())} {
				msgs := run(o, o.Open(tt.n))
				want := []tea.Msg{tt.want}
				if tt.toast != "" {
					want = append(want, ui.NotifyMsg{Level: toast.Info, Text: tt.toast})
				}
				if !slices.Equal(msgs, want) {
					t.Errorf("Open sent %#v, want %#v", msgs, want)
				}
			}
		})
	}
}

func TestRunFilter(t *testing.T) {
	for _, tt := range []struct {
		title string
		f     core.RunFilter
		ok    bool
	}{
		{"CI workflow run succeeded for main branch", core.RunFilter{Branch: "main", Status: "success"}, true},
		{"Build workflow run cancelled for release/v2 branch", core.RunFilter{Branch: "release/v2", Status: "cancelled"}, true},
		{"ci workflow run failed for feat/search-page branch", core.RunFilter{Branch: "feat/search-page", Status: "failure"}, true},
		{"CI run failed on main", core.RunFilter{}, false},
	} {
		if f, ok := runFilter(core.Subject{Type: core.SubjectCheckSuite, Title: tt.title}); f != tt.f || ok != tt.ok {
			t.Errorf("runFilter(%q) = %+v, %v", tt.title, f, ok)
		}
	}
}

func TestOpenMarksStaleWhatChanged(t *testing.T) {
	f := newReads()
	o := newOpener(t, f, 0, 0)
	run(o, o.Open(note(core.SubjectPullRequest, 1, time.Minute)))
	run(o, o.Open(note(core.SubjectIssue, 2, time.Hour)))
	run(o, o.Open(note(core.SubjectDiscussion, 3, time.Hour)))
	want := []string{"pull charmbracelet/glow#1 11:59:00", "issue charmbracelet/glow#2 11:00:00"}
	if !slices.Equal(f.changes, want) {
		t.Errorf("changes = %q, want %q", f.changes, want)
	}
}

func TestMarksRead(t *testing.T) {
	var none *Opener
	if !none.MarksRead() || !New(t.Context()).MarksRead() {
		t.Error("opening doesn't mark read by default")
	}
	if New(t.Context(), WithMarkRead(false)).MarksRead() {
		t.Error("WithMarkRead(false) still marks read")
	}
}

func TestReadAheadFirstRows(t *testing.T) {
	f := newReads()
	o := newOpener(t, f, 4, time.Hour)
	ns := []core.Notification{
		note(core.SubjectDiscussion, 1, time.Minute),
		note(core.SubjectPullRequest, 2, time.Minute),
		note(core.SubjectIssue, 3, time.Minute),
		note(core.SubjectCommit, 4, time.Minute),
		note(core.SubjectRelease, 5, time.Minute),
	}
	// The cursor is on the discussion, which has nothing to read.
	run(o, o.ReadAhead(list(ns), ns[0], true))
	// Of the first four rows, the pull request and the issue, each with
	// its first comments. The release is the fifth.
	want := []string{"issue charmbracelet/glow#3", "pull charmbracelet/glow#2"}
	if got := slices.Sorted(slices.Values(f.got())); !slices.Equal(got, want) {
		t.Errorf("read %q, want %q", got, want)
	}
	if got := slices.Sorted(slices.Values(f.comments)); !slices.Equal(got, want) {
		t.Errorf("read the first comments of %q, want %q", got, want)
	}

	// The same rows read nothing again.
	run(o, o.ReadAhead(list(ns), ns[0], true))
	if n := len(f.got()); n != 2 {
		t.Errorf("read %d details, want no more", n)
	}

	// A notification that the pull request changed since reads it again.
	ns[1].UpdatedAt = now.Add(time.Minute)
	run(o, o.ReadAhead(list(ns), ns[0], true))
	if got := f.got(); len(got) != 3 || got[2] != "pull charmbracelet/glow#2" {
		t.Errorf("read %q, want the pull request read again", got)
	}
}

func TestReadAheadHover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newReads()
		o := newOpener(t, f, 0, 150*time.Millisecond)
		ns := []core.Notification{note(core.SubjectRelease, 1, 0), note(core.SubjectCommit, 2, 0), note(core.SubjectIssue, 3, 0)}

		start := time.Now()
		run(o, o.ReadAhead(list(ns), ns[0], true))
		if waited := time.Since(start); waited != 150*time.Millisecond {
			t.Errorf("read after %v, want the delay", waited)
		}
		if got := f.got(); !slices.Equal(got, []string{"release charmbracelet/glow#1"}) {
			t.Errorf("read %q, want the release under the cursor", got)
		}
		// A commit has nothing to read ahead, and a row already read
		// reads nothing.
		run(o, o.ReadAhead(list(ns), ns[1], true))
		run(o, o.ReadAhead(list(ns), ns[0], true))
		if n := len(f.got()); n != 1 {
			t.Errorf("read %d, want no more", n)
		}
		// Moving on before the delay reads only where the cursor rests.
		first := o.ReadAhead(list(ns), ns[2], true)
		second := o.ReadAhead(list(ns), ns[1], true)
		run(o, tea.Batch(first, second))
		if n := len(f.got()); n != 1 {
			t.Errorf("read %q, want nothing for a row passed", f.got())
		}
	})
}

func TestReadAheadCancelledByReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newReads()
		f.hold = make(chan struct{})
		o := newOpener(t, f, 3, time.Hour)
		ns := []core.Notification{note(core.SubjectIssue, 1, 0), note(core.SubjectPullRequest, 2, 0)}
		cmd := o.ReadAhead(list(ns), core.Notification{}, false)
		done := make(chan struct{})
		go func() {
			run(o, cmd)
			close(done)
		}()
		synctest.Wait()
		ctxs := f.contexts()
		if len(ctxs) != 2 {
			t.Fatalf("%d reads in flight, want 2", len(ctxs))
		}
		o.Reset(context.Background())
		<-done
		for _, ctx := range ctxs {
			if ctx.Err() == nil {
				t.Error("a read ahead wasn't cancelled")
			}
		}
	})
}

// Leaving the screen stops the reads ahead in flight, and the view shown
// next reads its own rows through the same opener.
func TestStopCancelsReadsAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newReads()
		f.hold = make(chan struct{})
		o := newOpener(t, f, 3, time.Hour)
		ns := []core.Notification{note(core.SubjectIssue, 1, 0), note(core.SubjectPullRequest, 2, 0)}
		cmd := o.ReadAhead(list(ns), core.Notification{}, false)
		done := make(chan struct{})
		go func() {
			run(o, cmd)
			close(done)
		}()
		synctest.Wait()
		ctxs := f.contexts()
		if len(ctxs) != 2 {
			t.Fatalf("%d reads in flight, want 2", len(ctxs))
		}
		o.Stop()
		<-done
		for _, ctx := range ctxs {
			if ctx.Err() == nil {
				t.Error("a read ahead wasn't cancelled")
			}
		}

		close(f.hold)
		f.mu.Lock()
		f.hold = nil
		f.mu.Unlock()
		other := []core.Notification{note(core.SubjectIssue, 1, 0), note(core.SubjectIssue, 3, 0)}
		run(o, o.ReadAhead(list(other), core.Notification{}, false))
		if got := slices.Sorted(slices.Values(f.got()[2:])); !slices.Equal(got, []string{"issue charmbracelet/glow#1", "issue charmbracelet/glow#3"}) {
			t.Errorf("read %q after Stop, want the new rows, the cancelled one again", got)
		}
	})
}

// The views that share an opener share its rate limit: one that hits it
// stops the reads ahead of the other.
func TestSharedOpenerRateLimitStopsBoth(t *testing.T) {
	f := newReads()
	f.err = &core.RateLimitError{Reset: now}
	o := newOpener(t, f, 3, 0)
	screen := []core.Notification{note(core.SubjectIssue, 1, 0), note(core.SubjectIssue, 2, 0)}
	run(o, o.ReadAhead(list(screen), screen[0], true))
	n := len(f.got())
	if n == 0 {
		t.Fatal("nothing was read ahead")
	}
	// The screen leaves, and the dashboard's inbox shows other threads.
	o.Stop()
	inbox := []core.Notification{note(core.SubjectPullRequest, 7, 0), note(core.SubjectIssue, 8, 0)}
	run(o, o.ReadAhead(list(inbox), inbox[0], true))
	if got := len(f.got()); got != n {
		t.Errorf("read %d more for the other view under the rate limit", got-n)
	}
}

func TestReadAheadStopsAtRateLimit(t *testing.T) {
	f := newReads()
	f.err = &core.RateLimitError{Reset: now}
	o := newOpener(t, f, 5, 0)
	ns := make([]core.Notification, 0, 8)
	for i := range 8 {
		ns = append(ns, note(core.SubjectIssue, i+1, 0))
	}
	run(o, o.ReadAhead(list(ns), ns[0], true))
	n := len(f.got())
	if n == 0 || n > 3 {
		t.Errorf("read %d before the rate limit stopped it, want 1 to 3", n)
	}
	run(o, o.ReadAhead(list(ns[1:]), ns[6], true))
	if got := len(f.got()); got != n {
		t.Errorf("read %d more under the rate limit", got-n)
	}
	f.mu.Lock()
	f.err = nil
	f.mu.Unlock()
	o.Resume()
	run(o, o.ReadAhead(list(ns), ns[7], true))
	if got := len(f.got()); got == n {
		t.Error("nothing was read ahead after Resume")
	}
}

func TestNothingAheadWithoutPrefetch(t *testing.T) {
	f := newReads()
	o := New(t.Context(), WithPulls(fakePulls{f}), WithIssues(fakeIssues{f}))
	ns := []core.Notification{note(core.SubjectIssue, 1, 0)}
	if cmd := o.ReadAhead(list(ns), ns[0], true); cmd != nil {
		run(o, cmd)
	}
	var none *Opener
	if none.ReadAhead(list(ns), ns[0], true) != nil || none.Rested(ui.AheadMsg{}) != nil {
		t.Error("a nil Opener reads ahead")
	}
	if n := len(f.got()); n != 0 {
		t.Errorf("read %d without WithPrefetch", n)
	}
}
