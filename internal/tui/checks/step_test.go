package checks

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

func TestGroupsAndSummary(t *testing.T) {
	s, _ := newStep(t, newFake(), wideW, wideH)
	var got []string
	for _, r := range s.rows {
		if r.title() {
			got = append(got, "# "+r.group)
		} else {
			got = append(got, r.name())
		}
	}
	want := []string{
		"# CI", "test (ubuntu-latest)", "lint",
		"# Other checks", "codecov/patch",
		"# Build", "build",
		"# Statuses", "netlify/deploy", "ci/circleci",
	}
	if !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if v := text(s); !strings.Contains(v, "Checks ✗ 2 failing, ◐ 2 pending, ✓ 2 passed") {
		t.Errorf("the crumb doesn't count the checks:\n%s", v)
	}
	if v := text(s); !strings.Contains(v, "test (ubuntu-latest) required 3m 2s") || !strings.Contains(v, "netlify/deploy required Deploying the preview") {
		t.Errorf("the rows don't say what is required:\n%s", v)
	}
}

func TestOpensOnTheFirstFailingCheck(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, wideW, wideH)
	if r, _ := s.selected(); r.name() != "test (ubuntu-latest)" {
		t.Errorf("opens on %q, want the failing test", r.name())
	}
	// Moves skip the titles of the groups, and stop at the ends.
	h.keys("down", "down")
	if r, _ := s.selected(); r.name() != "codecov/patch" {
		t.Errorf("two down is %q, want the check after the title of its group", r.name())
	}
	h.keys("G", "down")
	if r, _ := s.selected(); r.name() != "ci/circleci" {
		t.Errorf("the end is %q", r.name())
	}
	h.keys("g", "up")
	if r, _ := s.selected(); r.name() != "test (ubuntu-latest)" {
		t.Errorf("the top is %q", r.name())
	}
	// A poll keeps the cursor on its check.
	h.keys("down")
	c := testChecks()
	c.Runs[1].Conclusion = core.ConclusionSuccess
	f.setChecks(c)
	h.send(ui.SyncMsg{Key: actionssvc.ChecksSyncKey(query)})
	if r, _ := s.selected(); r.name() != "lint" {
		t.Errorf("after a poll the cursor is on %q, want lint", r.name())
	}
	if f.checkReads != 1 {
		t.Errorf("read the checks %d times, want once", f.checkReads)
	}
}

func TestDrillIntoTheJobAndBack(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, wideW, wideH)
	h.keys("enter")
	if s.mode != jobMode || s.view.State() != jobview.Ready || s.view.Errors() != 1 {
		t.Fatalf("enter shows mode %d with the log in state %d, want the log of the job with its error", s.mode, s.view.State())
	}
	v := text(s)
	for _, want := range []string{"Checks › CI › test (ubuntu-latest)", "Annotations 2", "✗ tea_test.go:54 want a frame, got none", "Process completed with exit code 1."} {
		if !strings.Contains(v, want) {
			t.Errorf("the job lacks %q:\n%s", want, v)
		}
	}
	// The annotation opens its file at the head of the pull request, and
	// the preview returns to the modal.
	h.keys("A", "enter")
	want := ui.OpenFileMsg{Repo: repo, Path: "tea_test.go", Ref: "f00dcafe", Line: 54, Return: ret}
	if got := h.take(); len(got) != 1 || got[0] != want {
		t.Fatalf("enter on the annotation sent %v, want %v", got, want)
	}
	if !s.hidden {
		t.Error("the preview doesn't hide the step")
	}
	h.send(ui.ReopenedMsg{Modal: ret})
	if s.hidden || s.mode != jobMode {
		t.Error("the step isn't back on the job")
	}
	h.keys("A")
	h.keys("e", "o")
	if got := h.take(); len(got) != 1 || got[0] != (ui.OpenMsg{URL: "https://github.com/charmbracelet/bubbletea/actions/runs/4812/job/101"}) {
		t.Errorf("o sent %v, want the job", got)
	}
	h.keys("esc")
	if s.mode != listMode {
		t.Fatalf("esc from the job left mode %d", s.mode)
	}
	if r, _ := s.selected(); r.name() != "test (ubuntu-latest)" {
		t.Errorf("back on %q, want the check it left", r.name())
	}
	h.keys("esc")
	if got := h.take(); len(got) != 1 || got[0] != (CloseMsg{ID: s.ID()}) {
		t.Errorf("esc from the checks sent %v, want the step closed", got)
	}
}

func TestExternalCheckShowsWhatItReported(t *testing.T) {
	s, h := newStep(t, newFake(), wideW, wideH)
	h.keys("down", "down", "enter")
	if s.mode != detailMode {
		t.Fatalf("enter on codecov shows mode %d, want its detail", s.mode)
	}
	v := text(s)
	for _, want := range []string{"Checks › Other checks › codecov/patch", "62.50% of diff hit", "3 lines in tea.go are not covered.", "tea.go", "62.5%"} {
		if !strings.Contains(v, want) {
			t.Errorf("the detail lacks %q:\n%s", want, v)
		}
	}
	rendered := s.rendered
	s.SetSize(wideW, wideH)
	if s.rendered != rendered {
		t.Error("the markdown was rendered again at the same width")
	}
	s.SetSize(narrowW, narrowH)
	if s.rendered == rendered {
		t.Error("the markdown wasn't rendered again at another width")
	}
	h.keys("o")
	if got := h.take(); len(got) != 1 || got[0] != (ui.OpenMsg{URL: "https://codecov.io/gh/charmbracelet/bubbletea/pull/1500"}) {
		t.Errorf("o sent %v, want the app's page", got)
	}
	h.keys("esc", "G", "enter")
	if v := text(s); !strings.Contains(v, "Your tests passed") {
		t.Errorf("a status doesn't show its description:\n%s", v)
	}
}

// A detail cut short offers the check's page only if it has one, and says
// where it is.
func TestDetailOffersItsPage(t *testing.T) {
	s, _ := newStep(t, newFake(), wideW, wideH)
	for _, tt := range []struct{ url, want string }{
		{"", ""},
		{"https://github.com/o/r/runs/1", "o to open on GitHub"},
		{"https://codecov.io/gh/o/r/pull/1", "o to open its page"},
	} {
		s.check = row{check: &core.Check{DetailsURL: tt.url}}
		if got := s.openHint(); got != tt.want {
			t.Errorf("the hint for %q is %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestWatchesWhilePending(t *testing.T) {
	f := newFake()
	w := &watches{}
	s, h := newStep(t, f, wideW, wideH, WithWatch(w.watch), WithFollow(w.follow))
	key := actionssvc.ChecksSyncKey(query)
	if !slices.Equal(w.started, []string{key}) {
		t.Fatalf("watched %v, want the checks while some are pending", w.started)
	}
	// Following the job in progress, and dropping it back on the checks.
	h.keys("down", "down", "down", "enter")
	if s.mode != jobMode || !slices.Equal(w.followed, []int64{buildRun}) {
		t.Fatalf("mode %d following %v, want the run of the job in progress", s.mode, w.followed)
	}
	jobs := testJobs()[buildRun]
	jobs[0].Status, jobs[0].Conclusion = core.RunCompleted, core.ConclusionSuccess
	run := testRuns()[buildRun]
	run.Status, run.Conclusion = core.RunCompleted, core.ConclusionSuccess
	f.mu.Lock()
	f.jobs[buildRun], f.runs[buildRun] = jobs, run
	f.mu.Unlock()
	h.send(ui.SyncMsg{Key: actionssvc.RunSyncKey(repo, buildRun)})
	if s.view.State() != jobview.Ready || !slices.Equal(w.unfollowed, []int64{buildRun}) {
		t.Errorf("the job didn't load once done (state %d), or its run is still followed: %v", s.view.State(), w.unfollowed)
	}
	h.keys("esc")

	// Everything done: the polls stop.
	c := testChecks()
	c.Runs[2].Status, c.Runs[2].Conclusion = core.RunCompleted, core.ConclusionSuccess
	c.Statuses[1].State = "success"
	f.setChecks(c)
	h.send(ui.SyncMsg{Key: key})
	if !slices.Equal(w.stopped, []string{key}) {
		t.Errorf("stopped %v, want the checks once none is pending", w.stopped)
	}
	// Pending again, and the step closes.
	c.Runs[2].Status = core.RunQueued
	f.setChecks(c)
	h.send(ui.SyncMsg{Key: key})
	s.Close()
	if len(w.started) != 2 || len(w.stopped) != 2 || s.ctx.Err() == nil {
		t.Errorf("started %v and stopped %v, want the close to stop the watch and the reads", w.started, w.stopped)
	}
}

func TestRerunFailedJobs(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, wideW, wideH)
	h.keys("enter", "ctrl+r")
	if s.ask == nil || s.ask.Question != "Re-run 1 failed job of CI #4812?" {
		t.Fatalf("ctrl+r asked %+v", s.ask)
	}
	h.keys("y")
	if !slices.Equal(f.sent, []string{"rerun failed"}) {
		t.Errorf("sent %v, want the re-run", f.sent)
	}
	if j, _ := s.view.Job(); j.Status != core.RunQueued {
		t.Errorf("the job shows %q after the re-run, want it queued at once", j.Status)
	}
	if f.invalidated == 0 || f.checkReads < 2 {
		t.Errorf("the confirmed re-run didn't read the checks again: %d invalidations, %d reads", f.invalidated, f.checkReads)
	}
	// A run in progress can't be re-run.
	h.keys("esc", "down", "down", "down", "enter", "ctrl+r")
	if s.ask != nil || !strings.Contains(s.notice, "still running") {
		t.Errorf("re-run of a run in progress: ask %+v, notice %q", s.ask, s.notice)
	}
}

func TestRerunAnswers(t *testing.T) {
	tests := []struct {
		answers []string
		sent    bool
	}{
		{[]string{"y"}, true},
		{[]string{"n"}, false},
		{[]string{"esc"}, false},
		// A second yes, such as a repeated key, re-runs nothing more.
		{[]string{"y", "y"}, true},
		// Enter isn't a yes, and other keys leave the question open.
		{[]string{"enter", "r", "ctrl+r", "q", "n"}, false},
		{[]string{"enter", "y"}, true},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.answers, " "), func(t *testing.T) {
			f := newFake()
			s, h := newStep(t, f, wideW, wideH)
			h.keys("enter", "ctrl+r")
			if s.ask == nil || len(f.sent) != 0 {
				t.Fatalf("ctrl+r asked %+v and sent %v", s.ask, f.sent)
			}
			// The answers arrive before what the first starts runs.
			var cmds []tea.Cmd
			for _, k := range tt.answers {
				cmds = append(cmds, s.Update(press(k)))
			}
			for _, c := range cmds {
				h.run(c)
			}
			var want []string
			if tt.sent {
				want = []string{"rerun failed"}
			}
			if !slices.Equal(f.sent, want) || s.ask != nil {
				t.Errorf("sent %v with the question %+v open, want %v", f.sent, s.ask, want)
			}
		})
	}
}

func TestRerunAsksAgain(t *testing.T) {
	tests := []struct {
		name   string
		meddle func(s *Step, h *host, f *fake)
		want   string
	}{
		{
			name: "re-run elsewhere",
			meddle: func(s *Step, _ *host, _ *fake) {
				s.job.run.Status, s.job.run.Conclusion = core.RunQueued, ""
			},
			want: "CI #4812 changed meanwhile, so nothing was sent.",
		},
		{
			name: "another job failed",
			meddle: func(_ *Step, _ *host, f *fake) {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.jobs[ciRun][0].Conclusion = core.ConclusionFailure
			},
			want: "CI #4812 changed meanwhile, so nothing was sent.",
		},
		{
			name: "access lost",
			meddle: func(_ *Step, h *host, _ *fake) {
				h.send(ui.CapsMsg{Repo: repo, Caps: core.RepoCaps{Known: true, Permission: core.PermissionRead}})
			},
			want: "Re-running needs write access to " + repo.String() + ".",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			s, h := newStep(t, f, wideW, wideH)
			h.keys("enter", "ctrl+r")
			if s.ask == nil {
				t.Fatal("ctrl+r asked nothing")
			}
			tt.meddle(s, h, f)
			h.take()
			h.keys("y")
			want := ui.NotifyMsg{Level: toast.Info, Text: tt.want}
			if got := h.take(); len(f.sent) != 0 || !slices.Contains(got, tea.Msg(want)) {
				t.Errorf("sent %v and showed %v, want only %q", f.sent, got, tt.want)
			}
		})
	}
}

func TestErrorsAndEmpty(t *testing.T) {
	f := newFake()
	f.checksErr = errBoom
	s, h := newStep(t, f, wideW, wideH)
	if v := text(s); !strings.Contains(v, "Couldn't load the checks: boom · r to retry") {
		t.Errorf("the error isn't inline:\n%s", v)
	}
	f.checksErr = nil
	h.keys("r")
	if !s.loaded {
		t.Error("r didn't read the checks again")
	}
	f = newFake()
	f.checks = core.Checks{SHA: "f00dcafe"}
	s, _ = newStep(t, f, wideW, wideH)
	if v := text(s); !strings.Contains(v, "No checks have reported") {
		t.Errorf("no checks:\n%s", v)
	}
}

func TestMessagesOfOtherStepsAreIgnored(t *testing.T) {
	f := newFake()
	s, h := newStep(t, f, wideW, wideH)
	other, _ := newStep(t, f, wideW, wideH)
	h.send(checksMsg{id: other.id, err: errBoom})
	h.send(ui.DoneMsg{From: "Issues"})
	h.send(ui.ReopenedMsg{Modal: &retModal{}})
	if !s.loaded || s.err != nil || s.hidden {
		t.Error("the messages of another step changed this one")
	}
}

func TestRerunNeedsWriteAccess(t *testing.T) {
	read := core.RepoCaps{Known: true, Permission: core.PermissionRead}
	f := newFake()
	s, h := newStep(t, f, wideW, wideH, WithCaps(read))
	h.keys("enter")
	if got := keysOf(s.Help()); slices.Contains(got, "rerun failed") {
		t.Errorf("help offers a re-run with read access: %v", got)
	}
	h.keys("ctrl+r")
	want := ui.NotifyMsg{Level: toast.Info, Text: "Re-running needs write access to " + repo.String() + "."}
	if s.ask != nil || !slices.Contains(h.got, tea.Msg(want)) {
		t.Errorf("ctrl+r with read access asked %+v and sent %v, want the toast %q", s.ask, h.got, want.Text)
	}

	// Once the caps say the viewer may write, the re-run is offered.
	h.send(ui.CapsMsg{Repo: repo, Caps: core.RepoCaps{Known: true, Permission: core.PermissionWrite}})
	if got := keysOf(s.Help()); !slices.Contains(got, "rerun failed") {
		t.Errorf("help lacks the re-run with write access: %v", got)
	}
	h.keys("ctrl+r", "y")
	if !slices.Equal(f.sent, []string{"rerun failed"}) {
		t.Errorf("sent %v, want the re-run", f.sent)
	}
}

// keysOf returns what the enabled keys of km do, as help shows them.
func keysOf(km help.KeyMap) []string {
	var out []string
	for _, b := range km.ShortHelp() {
		if b.Enabled() {
			out = append(out, b.Help().Desc)
		}
	}
	return out
}

// A diagram in a detail shows as its head, whose offer links to it on
// mermaid.live, through the viewport and the step's frame.
func TestDetailLinksDiagram(t *testing.T) {
	s, _ := newStep(t, newFake(), wideW, wideH)
	s.openDetail(row{check: &core.Check{Name: "arch", Summary: "```mermaid\ngraph LR\n  a --> b\n```"}})
	v := s.View()
	if !strings.Contains(ansi.Strip(v), "◆ flowchart · 2 lines · View diagram ↗") ||
		strings.Count(v, "\x1b]8;;https://mermaid.live/view#pako:") != 1 || strings.Count(v, "\x1b]8;;\x1b\\") != 1 {
		t.Errorf("the detail doesn't link the diagram:\n%q", v)
	}
}
