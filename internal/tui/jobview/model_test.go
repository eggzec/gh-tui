package jobview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestShowsAFailedJobOnItsError(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 12)
	run(m, m.Show(failed(), false, Hints{}))
	if m.State() != Ready || m.Errors() != 1 || m.JobID() != failedJob {
		t.Fatalf("state %d with %d errors for job %d, want the log with its error", m.State(), m.Errors(), m.JobID())
	}
	if s := text(m); !strings.Contains(s, "Process completed with exit code 1.") {
		t.Errorf("the failed step isn't open on its error:\n%s", s)
	}
	// Showing it again, as a poll does, reads nothing.
	run(m, m.Show(failed(), false, Hints{}))
	if len(f.reads) != 1 {
		t.Errorf("read the log %d times, want once", len(f.reads))
	}
}

func TestStates(t *testing.T) {
	tests := []struct {
		name  string
		job   core.Job
		err   error
		state State
		text  string
	}{
		{"running", running(), nil, Pending, "Logs appear when the job finishes · o to watch live on GitHub ✓ Set up job 2s ◐ Run golangci-lint 1m 18s ○ Complete job queued"},
		{"not yet", failed(), fmt.Errorf("log: %w", core.ErrLogPending), Pending, "The job hasn't started yet."},
		{"expired", failed(), fmt.Errorf("log: %w", core.ErrLogExpired), Expired, "GitHub no longer keeps this log. ✓ Set up job 2s ✗ Run go test ./... 2m 58s"},
		{"failed", failed(), errBoom, Failed, "✗ Something went wrong"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.errs[failedJob] = tt.err
			m := newView(t, f, 80, 12)
			run(m, m.Show(tt.job, false, Hints{}))
			if m.State() != tt.state {
				t.Fatalf("state %d, want %d", m.State(), tt.state)
			}
			if s := text(m); !strings.Contains(s, tt.text) {
				t.Errorf("the view lacks %q:\n%s", tt.text, s)
			}
			assertFits(t, m.View(), 80, 12)
		})
	}
}

// The pending text points to GitHub only while the key that opens it
// works.
func TestPendingWithoutOpen(t *testing.T) {
	k := testKeys()
	k.Open.SetEnabled(false)
	m := New(t.Context(), newFake(), repo, k, WithClock(func() time.Time { return testNow }))
	m.SetTheme(testTheme())
	m.SetSize(80, 12)
	run(&m, m.Show(running(), false, Hints{}))
	if s := text(&m); !strings.Contains(s, "Logs appear when the job finishes.") || strings.Contains(s, "live") {
		t.Errorf("the pending text names a key that doesn't work:\n%s", s)
	}
}

func TestRetry(t *testing.T) {
	f := newFake()
	f.errs[failedJob] = errBoom
	m := newView(t, f, 80, 12)
	run(m, m.Show(failed(), false, Hints{}))
	if m.State() != Failed {
		t.Fatalf("state %d, want failed", m.State())
	}
	delete(f.errs, failedJob)
	run(m, m.Retry())
	if m.State() != Ready {
		t.Errorf("retry left state %d, want the log", m.State())
	}
	if m.Retry() != nil {
		t.Error("retry of a log that loaded reads it again")
	}
}

func TestRestWaitsUntilReadNow(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 12, WithRest(time.Hour))
	cmd := m.Show(failed(), true, Hints{})
	if m.State() != Loading || len(f.reads) != 0 {
		t.Fatalf("a job shown to rest is in state %d after %d reads, want loading, unread", m.State(), len(f.reads))
	}
	_ = cmd // The rest would end in an hour.
	run(m, m.ReadNow())
	if m.State() != Ready || !slices.Equal(f.reads, []int64{failedJob}) {
		t.Errorf("read now left state %d after reads %v", m.State(), f.reads)
	}
	// The rest in flight is stale now.
	*m, cmd = m.Update(restMsg{id: m.id, seq: m.seq - 1})
	if cmd != nil {
		t.Error("a stale rest reads")
	}
}

func TestFromMemory(t *testing.T) {
	f := newFake()
	f.cached[failedJob], f.cachedNotes[failedJob] = true, true
	m := newView(t, f, 80, 12)
	if cmd := m.Show(failed(), true, Hints{}); cmd != nil || m.State() != Ready {
		t.Errorf("a log in memory isn't shown at once: state %d", m.State())
	}
}

func TestPendingJobLoadsOnceDone(t *testing.T) {
	f := newFake()
	f.logs[runningJob] = core.Log{Lines: []core.LogLine{{Text: "0 issues.", Step: 2}}}
	m := newView(t, f, 80, 12)
	run(m, m.Show(running(), false, Hints{}))
	j := running()
	j.Steps[1].Status, j.Steps[1].Conclusion = core.RunCompleted, core.ConclusionSuccess
	j.Steps[1].CompletedAt = at(time.Second)
	run(m, m.Show(j, false, Hints{}))
	if m.State() != Pending || !strings.Contains(text(m), "✓ Run golangci-lint") {
		t.Errorf("the steps didn't move on: state %d\n%s", m.State(), text(m))
	}
	j.Status, j.Conclusion = core.RunCompleted, core.ConclusionSuccess
	run(m, m.Show(j, false, Hints{}))
	if m.State() != Ready || m.Lines() != 1 {
		t.Errorf("the log didn't load once the job was done: state %d", m.State())
	}
}

func TestTruncatedNotice(t *testing.T) {
	f := newFake()
	lg := testLog()
	lg.Truncated = true
	f.logs[failedJob] = lg
	m := newView(t, f, 80, 12)
	run(m, m.Show(failed(), false, Hints{}))
	if s := text(m); !strings.Contains(s, "Only the end of this log: it is too large to read whole. o opens it on GitHub.") {
		t.Errorf("a truncated log doesn't say so:\n%s", s)
	}
	assertFits(t, m.View(), 80, 12)
}

func TestMessagesOfOtherViewsAreIgnored(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 12)
	other := newView(t, f, 80, 12)
	run(m, m.Show(failed(), false, Hints{}))
	*m, _ = m.Update(logMsg{id: other.id, jobID: failedJob, err: errBoom})
	*m, _ = m.Update(restMsg{id: other.id, seq: m.seq})
	if m.State() != Ready {
		t.Error("the messages of another view changed this one")
	}
}

func TestClear(t *testing.T) {
	m := newView(t, newFake(), 40, 4)
	run(m, m.Show(failed(), false, Hints{}))
	m.Clear()
	if _, ok := m.Job(); ok || m.JobID() != 0 || m.Title() != "" {
		t.Error("a cleared view still shows its job")
	}
}

func TestView(t *testing.T) {
	tests := []struct {
		name    string
		job     core.Job
		partial bool
		w, h    int
	}{
		{"failed", failed(), false, 80, 12},
		{"running", running(), false, 60, 8},
		{"partial", running(), true, 60, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			if tt.partial {
				f.partial[runningJob] = partialLog(2, 5, 1)
			}
			m := newView(t, f, tt.w, tt.h)
			run(m, m.Show(tt.job, false, Hints{}))
			v := m.View()
			assertFits(t, v, tt.w, tt.h)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewFitsAnySize(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {1, 1}, {10, 3}, {200, 50}} {
		for _, j := range []core.Job{failed(), running()} {
			m := newView(t, newFake(), 80, 12)
			run(m, m.Show(j, false, Hints{}))
			m.SetSize(size[0], size[1])
			if size[1] == 0 {
				continue
			}
			assertFits(t, m.View(), size[0], size[1])
		}
	}
}

func TestAnnotations(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 20)
	run(m, m.Show(failed(), false, Hints{SHA: "f00d"}))
	s := text(m)
	for _, want := range []string{"Annotations 3 A to pick one", "✗ tea_test.go:54 want a frame, got none", "! key.go:12 unused: x is unused", "✗ Process completed with exit code 1."} {
		if !strings.Contains(s, want) {
			t.Errorf("the view lacks %q:\n%s", want, s)
		}
	}
	assertFits(t, m.View(), 80, 20)
	m.Focus()
	if got := keys(m, "A"); len(got) != 0 || !m.OnAnnotations() || m.view.Focused() {
		t.Fatal("A didn't give the keys to the annotations")
	}
	// Folding every step works from the annotations, which keep the keys.
	before := m.view.View()
	if got := keys(m, "*"); len(got) != 0 || !m.OnAnnotations() || m.view.Focused() || m.view.View() == before {
		t.Error("* didn't fold the steps of the log from the annotations")
	}
	keys(m, "*")
	got := keys(m, "j", "enter")
	want := ui.OpenFileMsg{Repo: repo, Path: "key.go", Ref: "f00d", Line: 12}
	if len(got) != 1 || got[0] != want {
		t.Errorf("enter sent %v, want %v", got, want)
	}
	if got := keys(m, "j", "enter"); len(got) != 0 {
		t.Errorf("enter on an annotation of the workflow sent %v", got)
	}
	keys(m, "A")
	if m.OnAnnotations() || !m.view.Focused() {
		t.Error("A didn't give the keys back to the log")
	}
	// Blurred and focused again, the log keeps the keys.
	m.Blur()
	m.Focus()
	if m.OnAnnotations() {
		t.Error("the annotations took the keys back")
	}
}

func TestAnnotationsAreReadOnlyForFailedJobs(t *testing.T) {
	f := newFake()
	f.logs[runningJob] = testLog()
	m := newView(t, f, 80, 20)
	j := running()
	j.Status, j.Conclusion = core.RunCompleted, core.ConclusionSuccess
	run(m, m.Show(j, false, Hints{}))
	run(m, m.Show(failed(), false, Hints{NoAnnotations: true}))
	if len(f.noteReads) != 0 || strings.Contains(text(m), "Annotations") {
		t.Errorf("read annotations of %v, want none", f.noteReads)
	}
	// From memory, with the log.
	f.cachedNotes[failedJob], f.cached[failedJob] = true, true
	m = newView(t, f, 80, 20)
	if cmd := m.Show(failed(), false, Hints{}); cmd != nil || !strings.Contains(text(m), "Annotations 3") {
		t.Error("the annotations in memory didn't show at once")
	}
}

func TestAnnotationsRetry(t *testing.T) {
	f := newFake()
	f.notesErr = errBoom
	m := newView(t, f, 80, 20)
	run(m, m.Show(failed(), false, Hints{}))
	if s := text(m); !strings.Contains(s, "Annotations ✗ Something went wrong") {
		t.Fatalf("the view doesn't say the annotations failed:\n%s", s)
	}
	f.notesErr = nil
	run(m, m.Retry())
	if s := text(m); !strings.Contains(s, "Annotations 3") {
		t.Errorf("retry didn't read the annotations again:\n%s", s)
	}
}

// TestAnnotationsKeptReadAgain checks that annotations served kept while
// GitHub rate limited their read are read again once, by RetryKept, and
// that annotations read fresh aren't.
func TestAnnotationsKeptReadAgain(t *testing.T) {
	f := newFake()
	f.notesLimited = true
	m := newView(t, f, 80, 20)
	run(m, m.Show(failed(), false, Hints{}))
	f.notesLimited = false
	reads := len(f.noteReads)
	run(m, m.RetryKept())
	run(m, m.RetryKept())
	if got := f.noteReads[reads:]; len(got) != 1 || got[0] != failedJob {
		t.Errorf("annotations read after two RetryKept = %v, want those of job %d once", got, failedJob)
	}
}

// The annotations say why they failed to load the way the user should
// read it, after their title, without the error's chain, request or status
// code.
func TestAnnotationErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"offline", fmt.Errorf("list annotations: github: GET /repos/o/r/check-runs/1/annotations: %w", core.ErrOffline), "Annotations ✗ Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("list annotations: github: 403 Forbidden: %w", core.ErrForbidden), "Annotations ✗ You don't have access to charmbracelet/bubbletea · o to open on GitHub"},
		{"not found", fmt.Errorf("list annotations: github: 404 Not Found: %w", core.ErrNotFound), "Annotations ✗ test (ubuntu-latest, 1.26) doesn't exist or is private."},
		{"internal", fmt.Errorf("list annotations: github: decode: %s", termtexttest.Hostile), "Annotations ✗ Something went wrong · r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.notesErr = tt.err
			m := newView(t, f, 120, 20, WithVoice(ui.NewVoice(config.Default().Keys, "")))
			run(m, m.Show(failed(), false, Hints{}))
			termtexttest.AssertClean(t, m.View(), 120)
			s := text(m)
			if !strings.Contains(s+" ", tt.want+" ") {
				t.Errorf("the view shows %q, want %q", s, tt.want)
			}
			for _, leak := range []string{"github:", "list annotations", "GET", "403", "404", "decode"} {
				if strings.Contains(s, leak) {
					t.Errorf("the view shows %q: %q", leak, s)
				}
			}
		})
	}
}

// Annotations that failed to load say why, in both themes.
func TestViewNotesFailed(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			f := newFake()
			f.notesErr = fmt.Errorf("list annotations: %w", core.ErrForbidden)
			m := newView(t, f, 60, 12, WithVoice(ui.NewVoice(config.Default().Keys, "")))
			p, err := config.Default().Palette(dark)
			if err != nil {
				t.Fatal(err)
			}
			m.SetTheme(ui.NewTheme(p, dark))
			run(m, m.Show(failed(), false, Hints{}))
			v := m.View()
			assertFits(t, v, 60, 12)
			golden.RequireEqual(t, v)
		})
	}
}

func TestKeyLayers(t *testing.T) {
	m := newView(t, newFake(), 80, 20)
	run(m, m.Show(failed(), false, Hints{}))
	m.Focus()
	names := func() string {
		bs := ui.Hints{Layers: m.KeyLayers()}.ShortHelp()
		out := make([]string, 0, len(bs))
		for _, b := range bs {
			out = append(out, b.Help().Key+" "+b.Help().Desc)
		}
		return strings.Join(out, ", ")
	}
	if got, want := names(), "↵ fold, * fold all, e next error, / search, q close, A annotations"; got != want {
		t.Errorf("keys of the log %q, want %q", got, want)
	}
	keys(m, "A")
	if got, want := names(), "* fold all, ↑/k up, ↓/j down, ↵ open file, A log"; got != want {
		t.Errorf("keys of the annotations %q, want %q", got, want)
	}
}

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, testKeys())
}

// The layers take a key in the order the view does: the log's until the
// annotations have the focus, and theirs alone then.
func TestKeyLayersOrder(t *testing.T) {
	m := newView(t, newFake(), 80, 20)
	run(m, m.Show(failed(), false, Hints{}))
	m.Focus()
	if _, src, _ := uitest.Winner(m.KeyLayers(), "j"); src != "log" {
		t.Errorf("j reaches %q, want the log", src)
	}
	keys(m, "A")
	if _, src, _ := uitest.Winner(m.KeyLayers(), "j"); src != "annotations" {
		t.Errorf("j reaches %q on the annotations, want them", src)
	}
	before := m.notes.cursor
	keys(m, "j")
	if m.notes.cursor != before+1 {
		t.Errorf("j moved the annotations from %d to %d, want one down", before, m.notes.cursor)
	}
}

// With the ASCII icons the job draws ASCII alone: its steps, the log's
// gutter and folds, and the annotations.
func TestViewASCII(t *testing.T) {
	for _, j := range []core.Job{failed(), running()} {
		f := newFake()
		m := newView(t, f, 80, 20, WithIcons(ui.NewIcons(config.IconsASCII)))
		run(m, m.Show(j, false, Hints{SHA: "f00d"}))
		if v := ansi.Strip(m.View()); strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("job %s: view isn't ASCII:\n%s", j.Name, v)
		}
	}
}

// TestReadLostReadsTheLogAgain checks that a log whose read was lost,
// while a preview hid the view, is read again at once, even one that
// waited for a rest, and that the rest in flight is stale then.
func TestReadLostReadsTheLogAgain(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%v", rest), func(t *testing.T) {
			f := newFake()
			m := newView(t, f, 80, 12, WithRest(time.Hour))
			_ = m.Show(failed(), rest, Hints{}) // Its answer goes to the preview.
			if m.State() != Loading {
				t.Fatalf("state %d, want loading", m.State())
			}
			seq := m.seq
			run(m, m.ReadLost())
			if m.State() != Ready || !slices.Equal(f.reads, []int64{failedJob}) {
				t.Errorf("read lost left state %d after reads %v, want the log read once", m.State(), f.reads)
			}
			var cmd tea.Cmd
			*m, cmd = m.Update(restMsg{id: m.id, seq: seq})
			if cmd != nil {
				t.Error("the rest from before the view was hidden reads")
			}
			if m.ReadLost() != nil {
				t.Error("read lost reads again a log that loaded")
			}
		})
	}
}
