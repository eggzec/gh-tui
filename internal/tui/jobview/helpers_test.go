package jobview

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

var (
	repo    = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	testNow = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
)

func at(d time.Duration) time.Time { return testNow.Add(-d) }

const (
	failedJob  = 101
	runningJob = 200
)

func failed() core.Job {
	return core.Job{
		ID: failedJob, RunID: 4812, Attempt: 1, Name: "test (ubuntu-latest, 1.26)", Status: core.RunCompleted,
		Conclusion: core.ConclusionFailure, StartedAt: at(time.Hour), CompletedAt: at(time.Hour - 3*time.Minute),
		Steps: []core.Step{
			{Number: 1, Name: "Set up job", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess, StartedAt: at(time.Hour), CompletedAt: at(time.Hour - 2*time.Second)},
			{Number: 2, Name: "Run go test ./...", Status: core.RunCompleted, Conclusion: core.ConclusionFailure, StartedAt: at(time.Hour - 2*time.Second), CompletedAt: at(time.Hour - 3*time.Minute)},
		},
	}
}

func running() core.Job {
	return core.Job{
		ID: runningJob, RunID: 4810, Attempt: 1, Name: "lint", Status: core.RunInProgress, StartedAt: at(80 * time.Second),
		Steps: []core.Step{
			{Number: 1, Name: "Set up job", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess, StartedAt: at(80 * time.Second), CompletedAt: at(78 * time.Second)},
			{Number: 2, Name: "Run golangci-lint", Status: core.RunInProgress, StartedAt: at(78 * time.Second)},
			// GitHub lists the steps that haven't started as pending.
			{Number: 3, Name: "Complete job", Status: core.RunPending},
		},
	}
}

func testLog() core.Log {
	line := func(text string, kind core.LogKind, st int) core.LogLine {
		return core.LogLine{Time: at(time.Hour), Text: text, Kind: kind, Step: st}
	}
	return core.Log{Lines: []core.LogLine{
		line("Current runner version: '2.337.0'", core.LogPlain, 1),
		line("go test ./...", core.LogPlain, 2),
		line("--- FAIL: TestProgram (0.31s)", core.LogPlain, 2),
		line("Process completed with exit code 1.", core.LogError, 2),
	}}
}

func testNotes() []core.Annotation {
	return []core.Annotation{
		{Path: "tea_test.go", StartLine: 54, Level: core.AnnotationFailure, Message: "want a frame, got none"},
		{Path: "key.go", StartLine: 12, Level: core.AnnotationWarning, Title: "unused", Message: "x is unused"},
		{Path: ".github", Level: core.AnnotationFailure, Message: "Process completed with exit code 1."},
	}
}

// fake serves logs and annotations from memory and counts its reads.
type fake struct {
	mu     sync.Mutex
	logs   map[int64]core.Log
	cached map[int64]bool
	errs   map[int64]error
	reads  []int64

	notes       map[int64][]core.Annotation
	cachedNotes map[int64]bool
	notesErr    error
	noteReads   []int64
	// notesLimited serves the annotations kept, as while GitHub rate
	// limits their read.
	notesLimited bool

	// partial holds the partial logs of jobs in progress, which a read
	// finds, and cachedPartial those in memory. watching counts the
	// watches of each.
	partial       map[int64]core.PartialLog
	cachedPartial map[int64]bool
	partialReads  []int64
	watching      map[int64]int
}

func newFake() *fake {
	return &fake{
		logs: map[int64]core.Log{failedJob: testLog()}, cached: map[int64]bool{}, errs: map[int64]error{},
		notes: map[int64][]core.Annotation{failedJob: testNotes()}, cachedNotes: map[int64]bool{},
		partial: map[int64]core.PartialLog{}, cachedPartial: map[int64]bool{}, watching: map[int64]int{},
	}
}

func (f *fake) CachedAnnotations(q actionssvc.AnnotationsQuery) (core.Page[core.Annotation], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return core.Page[core.Annotation]{Items: f.notes[q.CheckRunID]}, f.cachedNotes[q.CheckRunID]
}

func (f *fake) Annotations(_ context.Context, q actionssvc.AnnotationsQuery) (core.Page[core.Annotation], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.noteReads = append(f.noteReads, q.CheckRunID)
	if f.notesErr != nil {
		return core.Page[core.Annotation]{}, f.notesErr
	}
	f.cachedNotes[q.CheckRunID] = true
	return core.Page[core.Annotation]{Items: f.notes[q.CheckRunID], Limited: f.notesLimited}, nil
}

func (f *fake) CachedLog(_ core.RepoRef, jobID int64) (core.Log, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logs[jobID], f.cached[jobID]
}

func (f *fake) Log(_ context.Context, _ core.RepoRef, jobID int64) (core.Log, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, jobID)
	if err := f.errs[jobID]; err != nil {
		return core.Log{}, err
	}
	f.cached[jobID] = true
	return f.logs[jobID], nil
}

func (f *fake) CachedPartialLog(_ core.RepoRef, jobID int64) (core.PartialLog, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.partial[jobID], f.cachedPartial[jobID]
}

func (f *fake) PartialLog(_ context.Context, _ core.RepoRef, jobID int64) (core.PartialLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.partialReads = append(f.partialReads, jobID)
	l, ok := f.partial[jobID]
	if !ok {
		return core.PartialLog{}, core.ErrLogPending
	}
	f.cachedPartial[jobID] = true
	return l, nil
}

func (f *fake) WatchLog(_ core.RepoRef, _, jobID int64) func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.watching[jobID]++
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.watching[jobID]--
	}
}

// poll sets the partial log of the running job as a poll of its run reads
// it: into memory.
func (f *fake) poll(l core.PartialLog) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.partial[runningJob], f.cachedPartial[runningJob] = l, true
}

var errBoom = errors.New("boom")

func testTheme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

func testKeys() KeyMap {
	return KeyMap{
		Log:        logview.DefaultKeyMap(),
		LogContext: "actions_log", NotesContext: "actions_annotations",
		Annotations:      key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "annotations")),
		NotesAnnotations: key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "log")),
		Up:               key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:             key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Select:           key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "open file")),
		Open:             key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "browser")),
	}
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

// keys presses each key, runs what it returns, and keeps the messages for
// the app.
func keys(m *Model, ks ...string) []tea.Msg {
	var got []tea.Msg
	for _, k := range ks {
		var cmd tea.Cmd
		*m, cmd = m.Update(press(k))
		if cmd != nil {
			if msg := cmd(); msg != nil {
				got = append(got, msg)
			}
		}
	}
	return got
}

func newView(tb testing.TB, f Service, w, h int, opts ...Option) *Model {
	tb.Helper()
	opts = append([]Option{WithClock(func() time.Time { return testNow }), WithIcons(ui.NewIcons(config.IconsUnicode))}, opts...)
	m := New(tb.Context(), f, repo, testKeys(), opts...)
	m.SetTheme(testTheme())
	m.SetSize(w, h)
	return &m
}

// run runs cmd and feeds what it sends back to m, the way the app would.
func run(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil, spinner.TickMsg:
	case tea.BatchMsg:
		for _, c := range msg {
			run(m, c)
		}
	default:
		var next tea.Cmd
		*m, next = m.Update(msg)
		run(m, next)
	}
}

func text(m *Model) string {
	return strings.Join(strings.Fields(ansi.Strip(m.View())), " ")
}

func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Errorf("view has %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Errorf("line %d is %d wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}
