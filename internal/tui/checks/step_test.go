package checks

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
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

// A log that failed to load says what went wrong the way the user should
// read it, without the error's chain, request or status code.
func TestLogErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"offline", fmt.Errorf("job log: github: GET /repos/o/r/actions/jobs/1/logs: %w", core.ErrOffline), "✗ Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("job log: github: 403 Forbidden: %w", core.ErrForbidden), "✗ You don't have access to charmbracelet/bubbletea · o to open on GitHub"},
		{"internal", errors.New("job log: github: decode: unexpected EOF"), "✗ Something went wrong · r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.logErr = tt.err
			s, h := newStep(t, f, wideW, wideH)
			h.keys("enter")
			if v := strings.Join(strings.Fields(text(s)), " "); !strings.Contains(v, tt.want) || strings.Contains(v, "github:") || strings.Contains(v, "403") {
				t.Errorf("the job = %q, want %q", v, tt.want)
			}
		})
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

// A run with more jobs than a page lists finds the job of a check, and
// counts the failed jobs, on every page.
func TestJobsPastTheFirstPage(t *testing.T) {
	f := newFake()
	jobs := make([]core.Job, 0, actionssvc.DefaultJobPageSize+2)
	for i := range actionssvc.DefaultJobPageSize {
		c := core.ConclusionSuccess
		if i == 0 {
			c = core.ConclusionFailure
		}
		jobs = append(jobs, core.Job{ID: int64(1000 + i), RunID: ciRun, Attempt: 1, Name: "shard " + strconv.Itoa(i), Status: core.RunCompleted, Conclusion: c})
	}
	// The jobs of the checks come after the first page.
	f.jobs[ciRun] = append(jobs, f.jobs[ciRun]...)
	s, h := newStep(t, f, wideW, wideH)
	h.keys("enter")
	if j, ok := s.view.Job(); !ok || j.ID != testJob {
		t.Fatalf("the check shows job %d, want %d from the second page", j.ID, testJob)
	}
	h.keys("ctrl+r")
	if s.ask == nil || s.ask.Question != "Re-run 2 failed jobs of CI #4812?" {
		t.Errorf("ctrl+r asked %+v, want the failed jobs of both pages", s.ask)
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
	if v := text(s); !strings.Contains(v, "✗ Something went wrong · r to retry") {
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

// The checks and the job say what went wrong the way the user should read
// it, without the error's chain, request or status code. The open key
// opens the check of a job, but there is nothing to open without checks.
func TestErrorWords(t *testing.T) {
	pr := "charmbracelet/bubbletea#" + strconv.Itoa(query.Number)
	tests := []struct {
		name        string
		err         error
		checks, job string
	}{
		{
			"offline", fmt.Errorf("pull checks: github: POST /graphql: %w", core.ErrOffline),
			"✗ Can't reach GitHub · r to retry", "✗ Can't reach GitHub · r to retry",
		},
		{
			"forbidden", fmt.Errorf("pull checks: github: 403 Forbidden: %w", core.ErrForbidden),
			"✗ You don't have access to charmbracelet/bubbletea", "✗ You don't have access to charmbracelet/bubbletea · o to open on GitHub",
		},
		{
			"not found", fmt.Errorf("pull checks: github: 404 Not Found: %w", core.ErrNotFound),
			"✗ " + pr + " doesn't exist or is private.", "✗ test (ubuntu-latest) doesn't exist or is private.",
		},
		{
			"internal", fmt.Errorf("pull checks: github: decode: %s", termtexttest.Hostile),
			"✗ Something went wrong · r to retry", "✗ Something went wrong · r to retry",
		},
	}
	check := func(t *testing.T, s *Step, want string) {
		t.Helper()
		termtexttest.AssertClean(t, s.View(), wideW)
		v := text(s)
		if !strings.Contains(v+" ", want+" ") {
			t.Errorf("the step shows %q, want %q", v, want)
		}
		for _, hint := range []string{"to retry", "open on GitHub"} {
			if strings.Contains(v, hint) != strings.Contains(want, hint) {
				t.Errorf("the step shows %q, want %q", v, want)
			}
		}
		for _, leak := range []string{"github:", "pull checks", "POST", "403", "404", "decode"} {
			if strings.Contains(v, leak) {
				t.Errorf("the step shows %q: %q", leak, v)
			}
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.checksErr = tt.err
			s, _ := newStep(t, f, wideW, wideH)
			check(t, s, tt.checks)

			f = newFake()
			f.jobsErr = tt.err
			s, h := newStep(t, f, wideW, wideH)
			h.keys("enter")
			check(t, s, tt.job)
		})
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
	if got := uitest.Enabled(s.KeyLayers()); slices.Contains(got, "rerun failed") {
		t.Errorf("help offers a re-run with read access: %v", got)
	}
	h.keys("ctrl+r")
	want := ui.NotifyMsg{Level: toast.Info, Text: "Re-running needs write access to " + repo.String() + "."}
	if s.ask != nil || !slices.Contains(h.got, tea.Msg(want)) {
		t.Errorf("ctrl+r with read access asked %+v and sent %v, want the toast %q", s.ask, h.got, want.Text)
	}

	// Once the caps say the viewer may write, the re-run is offered.
	h.send(ui.CapsMsg{Repo: repo, Caps: core.RepoCaps{Known: true, Permission: core.PermissionWrite}})
	if got := uitest.Enabled(s.KeyLayers()); !slices.Contains(got, "rerun failed") {
		t.Errorf("help lacks the re-run with write access: %v", got)
	}
	h.keys("ctrl+r", "y")
	if !slices.Equal(f.sent, []string{"rerun failed"}) {
		t.Errorf("sent %v, want the re-run", f.sent)
	}
}

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
}

// The layers take a key in the order the step does: its own keys before
// those of the job it shows, and the answer alone while it asks.
func TestKeyLayersOrder(t *testing.T) {
	s, h := newStep(t, newFake(), wideW, wideH)
	if b, src, _ := uitest.Winner(s.KeyLayers(), "enter"); src != "checks" || b.Help().Desc != "log" {
		t.Errorf("enter reaches %q of %q, want the step's log", b.Help().Desc, src)
	}
	h.keys("enter")
	layers := s.KeyLayers()
	if b, src, _ := uitest.Winner(layers, "esc"); src != "checks" || b.Help().Desc != "checks" {
		t.Errorf("esc reaches %q of %q in the job, want the step's back to the checks", b.Help().Desc, src)
	}
	if _, src, _ := uitest.Winner(layers, "space"); src != "log" {
		t.Errorf("space reaches %q in the job, want the log", src)
	}
	// With a search of the log, esc clears it before it steps back.
	h.keys("/", "e", "x", "i", "t", "enter")
	if s.view.Query() == "" {
		t.Fatal("the search of the log didn't take")
	}
	if _, src, _ := uitest.Winner(s.KeyLayers(), "esc"); src != "log" {
		t.Errorf("esc reaches %q with a search, want the log", src)
	}
	h.keys("esc")
	if s.mode != jobMode || s.view.Query() != "" {
		t.Fatalf("esc with a search left mode %d and query %q, want the job without it", s.mode, s.view.Query())
	}
	layers = s.KeyLayers()
	if b, src, _ := uitest.Winner(layers, "ctrl+r"); src != "checks" || b.Help().Desc != "rerun failed" {
		t.Errorf("ctrl+r reaches %q of %q, want the re-run", b.Help().Desc, src)
	}
	h.keys("ctrl+r")
	if s.ask == nil {
		t.Fatal("ctrl+r didn't ask to re-run")
	}
	if _, src, _ := uitest.Winner(s.KeyLayers(), "esc"); src != "confirm" {
		t.Errorf("esc reaches %q while asking, want the answer", src)
	}
	h.keys("esc", "esc")
	if s.ask != nil || s.mode != listMode {
		t.Errorf("esc, esc left mode %d asking %v, want the checks", s.mode, s.ask != nil)
	}
}

// TestRerunKeyBeforeRefresh checks that ctrl+r, which refresh holds too,
// re-runs on the checks and in a job, and that the help names the re-run
// for it, while r refreshes. On a check an app reported, which can't be
// re-run, ctrl+r still doesn't refresh, and the help gives it to nothing.
func TestRerunKeyBeforeRefresh(t *testing.T) {
	const (
		rerun   = "re-run"
		refresh = "refresh"
		nothing = "nothing"
	)
	tests := []struct {
		name string
		to   []string // the keys that reach the list, a job or a detail
		key  string
		desc string // the binding the help names for key, if any
		// does is what the key does: re-run, which asks or says why
		// not, refresh, or nothing.
		does string
	}{
		{name: "ctrl+r on the checks", key: "ctrl+r", desc: "rerun failed", does: rerun},
		{name: "ctrl+r in a job", to: []string{"enter"}, key: "ctrl+r", desc: "rerun failed", does: rerun},
		{name: "r on the checks", key: "r", desc: "refresh", does: refresh},
		{name: "r in a job", to: []string{"enter"}, key: "r", desc: "refresh", does: refresh},
		{name: "ctrl+r on an app's check", to: []string{"down", "down"}, key: "ctrl+r", does: nothing},
		{name: "ctrl+r in an app's detail", to: []string{"down", "down", "enter"}, key: "ctrl+r", does: nothing},
		{name: "r on an app's check", to: []string{"down", "down"}, key: "r", desc: "refresh", does: refresh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			s, h := newStep(t, f, wideW, wideH)
			h.keys(tt.to...)
			b, src, ok := uitest.Winner(s.KeyLayers(), tt.key)
			if ok != (tt.desc != "") || ok && (src != "checks" || b.Help().Desc != tt.desc) {
				t.Errorf("the help gives %s to %q of %q, want %q of the step", tt.key, b.Help().Desc, src, tt.desc)
			}
			reads, jobReads := f.checkReads, f.jobReads
			h.keys(tt.key)
			did := nothing
			switch {
			case s.ask != nil || s.notice != "":
				did = rerun
			case f.checkReads > reads || f.jobReads > jobReads || f.invalidated > 0:
				did = refresh
			}
			// In a job, refresh retries the log, which reads nothing
			// that hasn't failed.
			if did != tt.does && (tt.does != refresh || s.mode != jobMode || did != nothing) {
				t.Errorf("%s did %s, want %s", tt.key, did, tt.does)
			}
		})
	}
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
