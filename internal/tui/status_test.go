package tui

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// fixedRates tells the rate limits it is set to. It is safe for
// concurrent use, as a program reads it while a test sets it.
type fixedRates struct {
	mu sync.Mutex
	s  core.RateStatus
}

func (r *fixedRates) RateStatus() core.RateStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.s
}

func (r *fixedRates) set(s core.RateStatus) {
	r.mu.Lock()
	r.s = s
	r.mu.Unlock()
}

// statusAt is when the rate statuses of the tests were taken.
var statusAt = time.Date(2026, 9, 28, 14, 0, 0, 0, time.Local)

// quotas returns core and graphql with what is left of them, resetting
// at 14:05 and 14:30.
func quotas(coreLeft, gqlLeft int) []core.Quota {
	return []core.Quota{
		{Resource: "core", Limit: 5000, Remaining: coreLeft, Reset: statusAt.Add(5 * time.Minute)},
		{Resource: "graphql", Limit: 5000, Remaining: gqlLeft, Reset: statusAt.Add(30 * time.Minute)},
		{Resource: "search", Limit: 30, Remaining: 30, Reset: statusAt.Add(time.Minute)},
	}
}

// statusCases are the states of the connection and the rate limits that
// the status bar tells.
func statusCases() []struct {
	name string
	s    core.RateStatus
} {
	answered := statusAt.Add(-time.Minute)
	limited := quotas(0, 4960)
	limited[0].LimitedUntil = statusAt.Add(5 * time.Minute)
	held := quotas(40, 4960)
	held[0].Held = 3
	return []struct {
		name string
		s    core.RateStatus
	}{
		{"unknown", core.RateStatus{At: statusAt}},
		{"online", core.RateStatus{Quotas: quotas(4812, 4960), Answered: answered, At: statusAt}},
		{"offline", core.RateStatus{Quotas: quotas(4812, 4960), Answered: answered, Failed: statusAt.Add(-30 * time.Second), At: statusAt}},
		{"limited", core.RateStatus{Quotas: limited, Answered: answered, At: statusAt}},
		{"secondary", core.RateStatus{Quotas: quotas(4812, 4960), SecondaryUntil: statusAt.Add(2 * time.Minute), Answered: answered, At: statusAt}},
		{"held", core.RateStatus{Quotas: held, Answered: answered, At: statusAt}},
		{"rejected", core.RateStatus{Answered: answered, Rejected: answered, At: statusAt}},
	}
}

// lastLine is the last line on screen.
func lastLine(m *Model) string {
	v := m.View().Content
	return v[strings.LastIndexByte(v, '\n')+1:]
}

func TestStatusBar(t *testing.T) {
	for _, c := range statusCases() {
		for _, dark := range []bool{false, true} {
			theme := "light"
			if dark {
				theme = "dark"
			}
			t.Run(c.name+"/"+theme, func(t *testing.T) {
				m, _ := newTestApp(t, WithRateStatus(&fixedRates{s: c.s}), WithLogin("laraibg786"), WithHost("github.com"))
				m.applyTheme(dark)
				var b strings.Builder
				for _, w := range []int{30, 60, 80, 120, 200} {
					m.Update(tea.WindowSizeMsg{Width: w, Height: 12})
					line := lastLine(m)
					if got := lipgloss.Width(line); got != w {
						t.Errorf("at %d columns the bar is %d wide", w, got)
					}
					b.WriteString(strconv.Itoa(w) + ": " + line + "\n")
				}
				golden.RequireEqual(t, b.String())
			})
		}
	}
}

func TestStatusBarStates(t *testing.T) {
	want := map[string]string{
		"unknown":   "laraibg786@github.com",
		"online":    "core 4 812/5 000 · gql 4 960/5 000 · resets 14:05 · ● online · laraibg786@github.com",
		"offline":   "● offline since 13:59",
		"limited":   "core 0/5 000 · gql 4 960/5 000 · resets 14:05 · ● rate limited until 14:05",
		"secondary": "● rate limited until 14:02",
		"held":      "resets 14:05 · 3 held · ● online",
		"rejected":  "● token rejected · laraibg786@github.com",
	}
	for _, c := range statusCases() {
		m, _ := newTestApp(t, WithRateStatus(&fixedRates{s: c.s}), WithLogin("laraibg786"), WithHost("github.com"))
		m.Update(tea.WindowSizeMsg{Width: 200, Height: 12})
		if line := ansi.Strip(lastLine(m)); !strings.Contains(line, want[c.name]) {
			t.Errorf("%s: the bar says\n%q\nwant it to say %q", c.name, line, want[c.name])
		}
	}
}

// TestStatusBarCompactRates checks that a narrow bar tells the tightest
// quota in percent.
func TestStatusBarCompactRates(t *testing.T) {
	s := core.RateStatus{Quotas: quotas(4812, 600), Answered: statusAt, At: statusAt}
	m, _ := newTestApp(t, WithRateStatus(&fixedRates{s: s}))
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 12})
	if line := ansi.Strip(lastLine(m)); !strings.Contains(line, "gql 12% · ● online") {
		t.Errorf("at 30 columns the bar says %q, want the tightest quota in percent", line)
	}
}

// TestStatusBarGivesWay checks the order the bar gives way in as the
// terminal narrows, at every width: every hint but "? help", the last
// first, then the account, then the rate limits, which shrink to the
// tightest quota first, then "? help", and the connection last, which
// shrinks to its dot first; and that a wider terminal never shows less.
func TestStatusBarGivesWay(t *testing.T) {
	s := core.RateStatus{Quotas: quotas(4812, 4960), Answered: statusAt, At: statusAt}
	m, _ := newTestApp(t, WithRateStatus(&fixedRates{s: s}), WithLogin("laraibg786"))
	// Each form, from the widest; a hint has one, and none is -1.
	const (
		rates = iota
		link
		account
	)
	var last []int
	for w := 200; w >= 1; w-- {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 12})
		line := ansi.Strip(lastLine(m))
		shown := m.status.Shown()
		hints, stats := shown[:len(m.hints)], shown[len(m.hints):]
		if len(stats) != 3 {
			t.Fatalf("width %d: %d items on the right, want the rates, the connection and the account", w, len(stats))
		}
		gone := func(forms ...int) bool {
			for _, f := range forms {
				if f != -1 {
					return false
				}
			}
			return true
		}
		if !strings.HasPrefix(ansi.Strip(m.hints[0].Forms[0]), "? help") {
			t.Fatalf("the first hint is %q, want ? help", ansi.Strip(m.hints[0].Forms[0]))
		}
		// The hints but the first go from the end.
		for i := 2; i < len(hints); i++ {
			if hints[i] == 0 && hints[i-1] == -1 {
				t.Errorf("width %d: hint %d shows after hint %d went: %q", w, i, i-1, line)
			}
		}
		help, others := hints[0], hints[1:]
		switch {
		case stats[account] == -1 && !gone(others...):
			t.Errorf("width %d: the account went before the hints: %q", w, line)
		case stats[rates] != 0 && stats[account] != -1:
			t.Errorf("width %d: the rate limits shrank before the account went: %q", w, line)
		case help == -1 && stats[rates] != -1:
			t.Errorf("width %d: ? help went before the rate limits: %q", w, line)
		case stats[link] != 0 && help != -1:
			t.Errorf("width %d: the connection shrank before ? help went: %q", w, line)
		}
		if last != nil {
			for i := range shown {
				if narrower(last[i], shown[i]) {
					t.Errorf("width %d shows more of item %d than width %d: %v, then %v", w, i, w+1, last, shown)
				}
			}
		}
		last = shown
	}
	// At the widest everything shows, and at one column nothing does.
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 12})
	for i, f := range m.status.Shown() {
		if f != 0 {
			t.Errorf("at 200 columns item %d shows form %d", i, f)
		}
	}
}

// narrower reports whether form a is narrower than form b, where -1 is
// none at all.
func narrower(a, b int) bool {
	if a == -1 {
		return b != -1
	}
	return b != -1 && a > b
}

// TestLineReplacesStatusBar checks that the command line takes the place
// of the status bar while it is open.
func TestLineReplacesStatusBar(t *testing.T) {
	m, _ := newTestApp(t, WithLogin("laraibg786"))
	if line := ansi.Strip(lastLine(m)); !strings.Contains(line, "? help") || !strings.Contains(line, "laraibg786@github.com") {
		t.Fatalf("the last line is %q, want the status bar", line)
	}
	drive(m, m.key(press(":")))
	if line := ansi.Strip(m.View().Content); strings.Contains(line, "? help") || strings.Contains(line, "laraibg786@github.com") {
		t.Errorf("the status bar shows with the command line open:\n%s", line)
	}
	drive(m, m.key(press("esc")))
	if line := ansi.Strip(lastLine(m)); !strings.Contains(line, "? help") {
		t.Errorf("the last line is %q once the command line closed, want the status bar", line)
	}
}

// TestFullHelpOverStatusBar checks that the help key shows every key over
// the status bar, and hides them again.
func TestFullHelpOverStatusBar(t *testing.T) {
	m, _ := newTestApp(t)
	drive(m, m.key(press("?")))
	v := ansi.Strip(m.View().Content)
	if !strings.Contains(v, "shift+tab previous pane") || !strings.Contains(ansi.Strip(lastLine(m)), "? help") {
		t.Errorf("? should show every key over the status bar:\n%s", v)
	}
	if got := strings.Count(v, "\n") + 1; got != 24 {
		t.Errorf("the screen is %d lines, want 24", got)
	}
	drive(m, m.key(press("?")))
	if v := ansi.Strip(m.View().Content); strings.Contains(v, "shift+tab previous pane") {
		t.Errorf("? again should hide the keys:\n%s", v)
	}
}

// TestToastsLeaveStatusBar checks that a toast never covers the status
// bar.
func TestToastsLeaveStatusBar(t *testing.T) {
	m, _ := newTestApp(t, WithLogin("laraibg786"))
	run(m, ui.Notify(toast.Success, "Merged #42"))
	if !hasToast(m, "Merged #42") || !strings.Contains(ansi.Strip(lastLine(m)), "laraibg786@github.com") {
		t.Errorf("the toast should show over the screen but not the bar:\n%s", ansi.Strip(m.View().Content))
	}
}

// TestProgramShowsRateLimitChanges checks that the bar tells the rate
// limits again when they change.
func TestProgramShowsRateLimitChanges(t *testing.T) {
	_, fakes := newTestApp(t)
	rates := &fixedRates{s: core.RateStatus{Quotas: quotas(4812, 4960), Answered: statusAt, At: statusAt}}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	app := New(t.Context(), config.Default(), layout, WithRepo(testRepo), WithRateStatus(rates))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(120, 24))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return drawn(out, "core 4 812/5 000")
	}, teatest.WithDuration(5*time.Second))
	offline := rates.RateStatus()
	offline.Quotas, offline.Failed = quotas(4000, 4960), statusAt.Add(time.Second)
	rates.set(offline)
	tm.Send(ui.SyncMsg{Key: core.SyncRateLimit})
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return drawn(out, "core 4 000/5 000") && drawn(out, "offline since 14:00")
	}, teatest.WithDuration(5*time.Second))
	tm.Send(ctrlC)
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}
