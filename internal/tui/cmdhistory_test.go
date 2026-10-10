package tui

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

// fakeHistory keeps lines in memory.
type fakeHistory struct {
	mu      sync.Mutex
	lines   []string
	loadErr error
	saveErr error
	saves   [][]string
}

func (f *fakeHistory) Load() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.lines), f.loadErr
}

func (f *fakeHistory) Save(lines []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves = append(f.saves, slices.Clone(lines))
	if f.saveErr != nil {
		return f.saveErr
	}
	f.lines = slices.Clone(lines)
	return nil
}

func (f *fakeHistory) saved() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.saves)
}

func TestHistoryIsLoadedAndSaved(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{lines: []string{"goto cli/cli", "goto #7"}}
	m, _ := newTestApp(t, WithCommandHistory(hist))
	drive(m, m.Init())
	drive(m, m.key(press(":")))
	drive(m, m.key(tea.KeyPressMsg{Code: tea.KeyUp}))
	if got := m.line.Value(); got != "goto #7" {
		t.Errorf("up recalls %q, want the last line kept", got)
	}
	drive(m, m.key(press("esc")))
	runCommand(t, m, "goto #8")
	want := [][]string{{"goto cli/cli", "goto #7", "goto #8"}}
	if got := hist.saved(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("saved %q, want %q", got, want)
	}
}

func TestHistoryWaitsForTheLoad(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{lines: []string{"goto cli/cli"}}
	m, _ := newTestApp(t, WithCommandHistory(hist))
	load := m.Init()
	runCommand(t, m, "goto #8")
	if got := hist.saved(); len(got) != 0 {
		t.Fatalf("saved %q before the lines kept were loaded", got)
	}
	drive(m, load)
	want := [][]string{{"goto cli/cli", "goto #8"}}
	if got := hist.saved(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("saved %q, want the lines kept and then the new one: %q", got, want)
	}
}

func TestHistoryFailuresAreQuiet(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{lines: []string{"goto cli/cli"}, loadErr: errors.New("corrupt"), saveErr: errors.New("read-only")}
	m, _ := newTestApp(t, WithCommandHistory(hist))
	drive(m, m.Init())
	if got := m.line.History(); len(got) != 0 {
		t.Errorf("history = %q after a failed load, want none", got)
	}
	runCommand(t, m, "goto #8")
	if got := m.line.History(); !slices.Equal(got, []string{"goto #8"}) {
		t.Errorf("history = %q, want the line of this session", got)
	}
	if s := toasted(m); s != "" {
		t.Errorf("a failure of the history showed a toast: %s", s)
	}
}

func TestHistorySavesInOrder(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{}
	m, _ := newTestApp(t, WithCommandHistory(hist))
	drive(m, m.Init())
	saves := make([]tea.Cmd, 0, 2)
	for _, line := range []string{"goto #1", "goto #2"} {
		drive(m, m.key(press(":")))
		typeKeys(m, line)
		// Enter adds the line to the history, before the app hears of it.
		m.key(enter)
		saves = append(saves, m.saveHistory())
	}
	// The later save runs first; the earlier is then out of date.
	saves[1]()
	saves[0]()
	if got := hist.saved(); len(got) != 1 || !slices.Equal(got[0], []string{"goto #1", "goto #2"}) {
		t.Errorf("saved %q, want only the later save", got)
	}
}

func TestQuitWaitsForTheLoad(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{lines: []string{"goto cli/cli"}}
	m, _ := newTestApp(t, WithCommandHistory(hist))
	load := m.Init()
	drive(m, m.key(press(":")))
	typeKeys(m, "q")
	msg := m.key(enter)()
	_, wait := m.Update(msg)
	if wait == nil || len(hist.saved()) != 0 {
		t.Fatalf("q should wait for the lines kept, and save nothing yet: saved %q", hist.saved())
	}
	msgs := drive(m, load)
	if !quit(msgs) {
		t.Error("q didn't quit once the lines kept were loaded")
	}
	want := [][]string{{"goto cli/cli", "q"}}
	if got := hist.saved(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("saved %q, want %q", got, want)
	}
	// The wait ends later, and changes nothing.
	if cmd := m.quitWaited(); cmd != nil {
		t.Error("the end of the wait quit again")
	}
}

func TestQuitWaitsNoLonger(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{lines: []string{"goto cli/cli"}}
	m, _ := newTestApp(t, WithCommandHistory(hist))
	m.Init()
	runLine := m.runLine("q", m.saveHistory())
	if runLine == nil {
		t.Fatal("q returned nothing to wait with")
	}
	cmd := m.quitWaited()
	if cmd == nil {
		t.Fatal("q didn't quit once it had waited")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("the end of the wait should quit")
	}
	if got := hist.saved(); len(got) != 0 {
		t.Errorf("saved %q over the lines never loaded", got)
	}
}

func TestProgramSavesHistoryBeforeQuitting(t *testing.T) {
	t.Parallel()
	hist := &fakeHistory{lines: []string{"goto cli/cli"}}
	_, fakes := newTestApp(t)
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	app := New(t.Context(), config.Default(), layout, WithRepo(testRepo), WithCommandHistory(hist))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(80, 24))
	// q, right after the start, waits for the lines kept, saves them with
	// itself, and only then quits.
	for _, k := range []tea.KeyPressMsg{press(":"), press("q"), enter} {
		tm.Send(k)
	}
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	want := [][]string{{"goto cli/cli", "q"}}
	if got := hist.saved(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("saved %q, want %q before quitting", got, want)
	}
}
