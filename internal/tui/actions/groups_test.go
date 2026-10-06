package actions

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// shape writes nodes as "job" and "group{kid | kid}", to compare trees.
func shape(nodes []jobNode) string {
	parts := make([]string, len(nodes))
	for i, n := range nodes {
		parts[i] = n.label
		if n.group != nil {
			parts[i] += "{" + shape(n.group.kids) + "}"
		}
	}
	return strings.Join(parts, " | ")
}

func named(names ...string) []core.Job {
	jobs := make([]core.Job, len(names))
	for i, n := range names {
		jobs[i] = core.Job{ID: int64(i + 1), Name: n, Status: core.RunCompleted, Conclusion: core.ConclusionSuccess}
	}
	return jobs
}

func TestGroupJobs(t *testing.T) {
	tests := []struct {
		name  string
		jobs  []string
		shape string
	}{
		{"no groups", []string{"lint", "build"}, "lint | build"},
		{
			"callers",
			[]string{"build / linux", "lint", "build / osx", "docs / site", "docs / api"},
			"build{linux | osx} | lint | docs{site | api}",
		},
		{
			"matrix",
			[]string{"test (linux, amd64)", "lint", "test (osx, arm64)"},
			"test{linux, amd64 | osx, arm64} | lint",
		},
		{
			"custom matrix names",
			[]string{"Deploy to nightly (linux_amd64)", "Deploy to nightly (osx_arm64)", "Clang-Tidy (1/4)", "Clang-Tidy (2/4)"},
			"Deploy to nightly{linux_amd64 | osx_arm64} | Clang-Tidy{1/4 | 2/4}",
		},
		{
			"a base with parentheses of its own",
			[]string{"Tests (2nd batch) (linux)", "Tests (2nd batch) (osx)", "Tests (linux)"},
			"Tests (2nd batch){linux | osx} | Tests (linux)",
		},
		{
			"singletons stay whole",
			[]string{"notify / go", "test (linux)", "vars"},
			"notify / go | test (linux) | vars",
		},
		{
			"nested callers keep the rest of the name",
			[]string{
				"extensions / Main / Build / Linux (amd64)", "extensions / Main / Build / Linux (arm64)",
				"extensions / vars", "extensions / Main / Build / Generate matrix",
			},
			"extensions{Main / Build / Linux{amd64 | arm64} | vars | Main / Build / Generate matrix}",
		},
		{
			"callers every job shares",
			[]string{"Main / Build / Generate matrix", "Main / Build / Linux (amd64)", "Main / Build / Linux (arm64)", "lint"},
			"Main / Build{Generate matrix | Linux{amd64 | arm64}} | lint",
		},
		{
			"mixed",
			[]string{"check-draft", "Build / linux_amd64", "Test (linux)", "Build / wasm (mvp)", "Build / wasm (eh)", "Test (osx)", "Build / osx"},
			"check-draft | Build{linux_amd64 | wasm{mvp | eh} | osx} | Test{linux | osx}",
		},
		{
			"names GitHub cut",
			[]string{"Windows (windows_amd64, windows-latest, x64-windows-st...", "Windows (windows_arm64)"},
			"Windows{windows_amd64, windows-latest, x64-windows-st... | windows_arm64}",
		},
		{
			"a skipped job of a matrix not expanded joins it",
			[]string{"MacOS (osx_amd64)", "MacOS", "MacOS (osx_arm64)", "Wasm"},
			"MacOS{osx_amd64 | MacOS | osx_arm64} | Wasm",
		},
		{
			"expressions GitHub left in",
			[]string{"BWC Test (DuckDB ${{ matrix.series.group }})", "BWC Test (v1.1)", "Lint ${{matrix.os}}"},
			"BWC Test{DuckDB {series.group} | v1.1} | Lint {os}",
		},
		{
			"a caller and a matrix of the same name stay apart",
			[]string{"build / a", "build (x)", "build / b", "build (y)"},
			"build{a | b} | build{x | y}",
		},
		{"not a matrix", []string{"(x)", "a (", "b ()", "c(x)", "c(y)"}, "(x) | a ( | b () | c(x) | c(y)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shape(groupJobs(named(tt.jobs...))); got != tt.shape {
				t.Errorf("groups\n %s\nwant\n %s", got, tt.shape)
			}
		})
	}
}

// A group's glyph shows its worst job, and it opens on its first failed
// one.
func TestGroupState(t *testing.T) {
	done := func(c core.Conclusion) core.Job { return core.Job{Status: core.RunCompleted, Conclusion: c} }
	of := func(s core.RunStatus) core.Job { return core.Job{Status: s} }
	tests := []struct {
		name  string
		jobs  []core.Job
		state ui.RunState
		first int
	}{
		{"all passed", []core.Job{done(core.ConclusionSuccess), done(core.ConclusionSuccess)}, ui.RunSuccess, 0},
		{"skipped and passed", []core.Job{done(core.ConclusionSkipped), done(core.ConclusionSuccess)}, ui.RunSuccess, 0},
		{"all skipped", []core.Job{done(core.ConclusionSkipped), done(core.ConclusionSkipped)}, ui.RunSkipped, 0},
		{"one runs", []core.Job{done(core.ConclusionSuccess), of(core.RunQueued), of(core.RunInProgress)}, ui.RunInProgress, 2},
		{"waits", []core.Job{of(core.RunQueued), of(core.RunWaiting)}, ui.RunWaiting, 0},
		{"cancelled over running", []core.Job{of(core.RunInProgress), done(core.ConclusionCancelled)}, ui.RunCancelled, 0},
		{"failed over all", []core.Job{done(core.ConclusionCancelled), of(core.RunInProgress), done(core.ConclusionFailure)}, ui.RunFailure, 2},
		{"timed out", []core.Job{done(core.ConclusionSuccess), done(core.ConclusionTimedOut)}, ui.RunTimedOut, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.jobs {
				tt.jobs[i].Name = fmt.Sprintf("m (%d)", i)
			}
			nodes := groupJobs(tt.jobs)
			if len(nodes) != 1 || nodes[0].group == nil {
				t.Fatalf("groups %s, want one", shape(nodes))
			}
			if g := nodes[0].group; g.state != tt.state || g.first != tt.first {
				t.Errorf("state %d opening on %d, want %d on %d", g.state, g.first, tt.state, tt.first)
			}
		})
	}
}

// duckJobs are the jobs of a run shaped like DuckDB's CI: reusable
// workflows, matrices, and a skipped matrix job GitHub never expanded.
func duckJobs(runID int64) []core.Job {
	var jobs []core.Job
	add := func(name string, status core.RunStatus, c core.Conclusion) {
		id := int64(9000 + len(jobs))
		j := core.Job{
			ID: id, RunID: runID, Attempt: 1, Name: name, Status: status, Conclusion: c,
			StartedAt: at(20 * time.Minute), URL: fmt.Sprintf("https://github.com/duckdb/duckdb/actions/runs/%d/job/%d", runID, id),
		}
		if status == core.RunCompleted {
			j.CompletedAt = j.StartedAt.Add(time.Duration(len(jobs)+1) * time.Minute)
		}
		jobs = append(jobs, j)
	}
	ok, skip, fail := core.ConclusionSuccess, core.ConclusionSkipped, core.ConclusionFailure
	done := core.RunCompleted
	add("check-draft", done, ok)
	for _, p := range []string{"linux_amd64", "linux_arm64", "osx", "windows_amd64"} {
		c := ok
		if p == "windows_amd64" {
			c = fail
		}
		add("Build extension binaries / "+p, done, c)
	}
	for _, w := range []string{"wasm_mvp", "wasm_eh", "wasm_threads"} {
		add("Build extension binaries / DuckDB-Wasm ("+w+", wasm32-emscripten)", done, ok)
	}
	add("Test (linux, amd64)", done, ok)
	add("Test (osx, arm64)", core.RunInProgress, core.ConclusionNone)
	add("Test (windows, amd64)", core.RunQueued, core.ConclusionNone)
	for i := range 4 {
		add(fmt.Sprintf("Clang-Tidy (%d/4)", i+1), done, ok)
	}
	add("BWC Test (DuckDB ${{ matrix.series.group }})", done, skip)
	add("extensions / Main Extensions / Build / Linux (linux_amd64, ubuntu-24.04, x64-linux-release, x64-linux-release, true, false)", done, ok)
	add("extensions / Main Extensions / Build / Linux (linux_arm64, ubuntu-24.04-arm, arm64-linux-release, arm64-linux-release, false, false)", done, ok)
	add("extensions / Main Extensions / Build / Windows (windows_amd64, windows-latest, x64-windows-static-release, x64-windows-static-release, t...", done, ok)
	add("extensions / Rust-based Extensions / Build / MacOS (osx_arm64, macos-15, arm64)", done, ok)
	add("extensions / Rust-based Extensions / Build / MacOS", done, skip)
	add("extensions / Upload Extensions", core.RunQueued, core.ConclusionNone)
	add("Deploy to nightly (linux_amd64)", core.RunWaiting, core.ConclusionNone)
	add("Deploy to nightly (osx_arm64)", core.RunWaiting, core.ConclusionNone)
	add("Upload Extensions", core.RunQueued, core.ConclusionNone)
	return jobs
}

const duckRun = 36340148524

// duckFake serves one run of DuckDB's shape, which failed on one job and
// still runs others.
func duckFake() *fake {
	f := newFake()
	f.runs = []core.Run{{
		ID: duckRun, Attempt: 1, Name: "InvokeCI", DisplayTitle: "Nightly", Number: 5120, Event: "schedule",
		Branch: "main", Status: core.RunInProgress, Actor: "duckdblabs-bot", WorkflowID: 9,
		CreatedAt: at(30 * time.Minute), RunStartedAt: at(30 * time.Minute), UpdatedAt: at(time.Minute),
		URL: "https://github.com/duckdb/duckdb/actions/runs/36340148524",
	}}
	f.jobs = map[int64][]core.Job{duckRun: duckJobs(duckRun)}
	return f
}

// jobsPaneRows is the rows of the jobs pane, without styles, trimmed.
func jobsPaneRows(m *Modal) []string {
	lines := m.paneLines(jobsPane, m.paneWidth(jobsPane), m.bodyHeight())
	var rows []string
	for _, l := range lines {
		if s := strings.TrimSpace(ansi.Strip(l)); s != "" {
			rows = append(rows, strings.Join(strings.Fields(s), " "))
		}
	}
	return rows
}

func TestViewJobGroups(t *testing.T) {
	// The sizes inside the frame on terminals of 80 by 24 and 120 by 30.
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{"80 columns", narrowW, narrowH, []string{"enter"}},
		{"120 columns", 92, 24, []string{"enter"}},
		// The extensions open, to show a group in a group.
		{"120 columns nested", 92, 24, []string{"enter", "G", "k", "k", "enter", "j", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, h := newModal(t, duckFake(), tt.width, tt.height)
			h.keys(tt.keys...)
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

func TestJobGroupsFold(t *testing.T) {
	m, h := newModal(t, duckFake(), wideW, wideH)
	h.keys("tab")
	if b, _, _ := uitest.Winner(m.KeyLayers(), "enter"); b.Help().Desc != "open" {
		t.Errorf("enter on a job reads %q, want open", b.Help().Desc)
	}
	// The failed group opens, on its failed job; the rest stay folded.
	want := []string{
		"✓ check-draft 1m 0s",
		"▾ Build extension binaries 7 ✗",
		"✓ linux_amd64 2m 0s",
		"✓ linux_arm64 3m 0s",
		"✓ osx 4m 0s",
		"▌ ✗ windows_amd64 5m 0s",
		"▸ DuckDB-Wasm 3 ✓",
		"▸ Test 3 ◐",
		"▸ Clang-Tidy 4 ✓",
		"⊖ BWC Test (DuckDB {s… skipped",
		"▸ extensions 6 ○",
		"▸ Deploy to nightly 2 ◷",
		"○ Upload Extensions queued",
	}
	if got := jobsPaneRows(m); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the jobs:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// On a group, the select key opens it, and the log shows its first
	// job that runs.
	h.keys("j", "j")
	if !m.jobs.onGroup() || m.log.JobID() != 9009 {
		t.Fatalf("on %q the log shows job %d, want the group's running job", jobsPaneRows(m)[7], m.log.JobID())
	}
	// The help says so, as it says that enter opens a job.
	if b, src, _ := uitest.Winner(m.KeyLayers(), "enter"); src != "Jobs" || b.Help().Desc != "fold" {
		t.Errorf("enter on a group reaches %q of %q, want the fold", b.Help().Desc, src)
	}
	h.keys("enter")
	if rows := jobsPaneRows(m); rows[7] != "▌ ▾ Test 3 ◐" || rows[8] != "✓ linux, amd64 9m 0s" || m.focus != jobsPane {
		t.Errorf("after enter on Test:\n%s", strings.Join(rows, "\n"))
	}
	h.keys("enter")
	if rows := jobsPaneRows(m); rows[8] != "▸ Clang-Tidy 4 ✓" {
		t.Errorf("after enter again Test isn't folded:\n%s", strings.Join(rows, "\n"))
	}
	// A group has no page of its own, so the browser opens the run.
	h.take()
	h.keys("o")
	if got := h.take(); len(got) != 1 || got[0] != (ui.OpenMsg{URL: m.run.URL}) {
		t.Errorf("o on a group sends %v, want the run's page", got)
	}
}

// A group isn't re-run as the one job it shows.
func TestJobGroupNotRerun(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("tab", "k", "J")
	if !m.jobs.onGroup() || m.ask != nil || lastLine(m) != "Pick a job of the group to re-run." {
		t.Errorf("re-running a group asks %v, says %q", m.ask, lastLine(m))
	}
	h.keys("j", "J")
	if m.ask == nil || !strings.Contains(m.ask.Question, "test (ubuntu-latest, 1.26)") {
		t.Errorf("re-running a job of a group asks %v", m.ask)
	}
}

// The folds and the cursor stay as they were while polls add jobs, and
// pages of them.
func TestJobGroupsSurvivePolls(t *testing.T) {
	f := duckFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab")
	// Fold the failed group, open the tests, and rest on Clang-Tidy.
	h.keys("k", "k", "k", "k", "enter", "j", "enter", "j", "j", "j", "j")
	before := jobsPaneRows(m)
	if !strings.HasPrefix(before[6], "▌ ▸ Clang-Tidy") {
		t.Fatalf("the cursor isn't on Clang-Tidy:\n%s", strings.Join(before, "\n"))
	}
	jobs := duckJobs(duckRun)
	extra := func(name string) core.Job {
		j := jobs[0]
		j.ID, j.Name = int64(9500+len(jobs)), name
		return j
	}
	// A job before the cursor, one in a folded group and one in an open
	// one, a new group, and a second page.
	jobs = append([]core.Job{extra("prepare")}, jobs...)
	jobs = append(jobs, extra("Build extension binaries / freebsd"), extra("Test (linux, arm64)"),
		extra("Python (3.12)"), extra("Python (3.13)"))
	f.moreJobs = true
	f.setJobs(duckRun, jobs)
	h.run(m.fromCache())
	want := []string{
		"✓ prepare 1m 0s",
		"✓ check-draft 1m 0s",
		"▸ Build extension binaries 8 ✗",
		"▾ Test 4 ◐",
		"✓ linux, amd64 9m 0s",
		"◐ osx, arm64 20m 0s",
		"○ windows, amd64 queued",
		"✓ linux, arm64 1m 0s",
		"▌ ▸ Clang-Tidy 4 ✓",
		"⊖ BWC Test (DuckDB {s… skipped",
		"▸ extensions 6 ○",
		"▸ Deploy to nightly 2 ◷",
		"○ Upload Extensions queued",
		"▸ Python 2 ✓",
		fmt.Sprintf("First %d jobs · o shows all", len(jobs)),
	}
	if got := jobsPaneRows(m); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("after the poll:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// A new attempt keeps the folds too, by the names of the groups.
	jobs = slices.Clone(jobs)
	for i := range jobs {
		jobs[i] = attempt(jobs[i], 2)
		jobs[i].ID += 1000
	}
	f.setJobs(duckRun, append(f.jobs[duckRun], jobs...))
	r := f.runs[0]
	r.Attempt = 2
	f.setRun(r)
	h.run(m.fromCache())
	if got := jobsPaneRows(m); got[2] != "▸ Build extension binaries 8 ✗" || got[8] != "▌ ▸ Clang-Tidy 4 ✓" {
		t.Errorf("after a new attempt:\n%s", strings.Join(got, "\n"))
	}
}

// A job the cursor was on stays under it, or under its group once folded.
func TestJobGroupsCursorFollowsItsJob(t *testing.T) {
	f := duckFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab")
	if j, _ := m.jobs.selected(); j.Name != "Build extension binaries / windows_amd64" {
		t.Fatalf("opens on %q, want the failed job", j.Name)
	}
	// The failed job passes on a new attempt, so its group opens no more
	// by itself, but it was open, so it stays open.
	jobs := duckJobs(duckRun)
	for i := range jobs {
		jobs[i] = attempt(jobs[i], 2)
		jobs[i].ID += 1000
		jobs[i].Conclusion = core.ConclusionSuccess
	}
	f.setJobs(duckRun, append(f.jobs[duckRun], jobs...))
	r := f.runs[0]
	r.Attempt = 2
	f.setRun(r)
	h.run(m.fromCache())
	if j, _ := m.jobs.selected(); j.ID != 10004 || m.jobs.onGroup() {
		t.Errorf("after a new attempt the cursor is on %q (%d), want the job of the same name", j.Name, j.ID)
	}
	// Folded, the group holds the cursor, and shows the same job.
	m.jobs.folds["Build extension binaries /"] = fold{}
	m.jobs.relines()
	m.jobs.cursor = m.jobs.lineOf(4)
	if !m.jobs.onGroup() || jobsPaneRows(m)[1] != "▌ ▸ Build extension binaries 7 ✓" {
		t.Errorf("the folded group:\n%s", strings.Join(jobsPaneRows(m), "\n"))
	}
}

// A long name gives way to the count and the state of its group.
func TestJobGroupLongName(t *testing.T) {
	f := newFake()
	long := strings.Repeat("Build every extension binary ", 3)
	f.jobs[failedRun] = []core.Job{
		job(1, failedRun, long+"(linux)", core.ConclusionFailure, time.Hour, time.Minute),
		job(2, failedRun, long+"(osx)", core.ConclusionSuccess, time.Hour, time.Minute),
	}
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab")
	row := jobsPaneRows(m)[0]
	if !strings.HasSuffix(row, "… 2 ✗") || !strings.HasPrefix(row, "▾ Build every") {
		t.Errorf("the long group's row is %q", row)
	}
	lines := m.paneLines(jobsPane, m.paneWidth(jobsPane), m.bodyHeight())
	if strings.Contains(lines[0], "\x1b]8;") || !strings.Contains(lines[1], "\x1b]8;;"+f.jobs[failedRun][0].URL) {
		t.Errorf("the group links, or its job doesn't:\n%q\n%q", lines[0], lines[1])
	}
}

// A job a poll makes a group of, with the jobs that follow it, stays under
// the cursor, and its log stays.
func TestJobGroupFromALoneJob(t *testing.T) {
	f := newFake()
	gen := job(1, failedRun, "Build / Generate matrix", core.ConclusionSuccess, time.Hour, time.Minute)
	f.jobs[failedRun] = []core.Job{
		job(2, failedRun, "check-draft", core.ConclusionSuccess, time.Hour, time.Minute),
		gen,
		job(3, failedRun, "lint", core.ConclusionFailure, time.Hour, time.Minute),
	}
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab", "k", "tab")
	if j, _ := m.jobs.selected(); j.ID != gen.ID || m.log.JobID() != gen.ID || m.focus != logPane {
		t.Fatalf("the log shows job %d, want %d", m.log.JobID(), gen.ID)
	}
	f.setJobs(failedRun, append(slices.Clone(f.jobs[failedRun]),
		job(4, failedRun, "Build / linux_amd64", core.ConclusionSuccess, time.Hour, time.Minute),
		job(5, failedRun, "Build / osx", core.ConclusionSuccess, time.Hour, time.Minute)))
	h.run(m.fromCache())
	if j, _ := m.jobs.selected(); j.ID != gen.ID || m.jobs.onGroup() || m.log.JobID() != gen.ID {
		t.Errorf("after the poll the cursor is on %q and the log shows job %d, want %d", j.Name, m.log.JobID(), gen.ID)
	}
	if rows := jobsPaneRows(m); rows[1] != "▾ Build 3 ✓" || !strings.HasPrefix(rows[2], "▌ ✓ Generate matrix") {
		t.Errorf("after the poll:\n%s", strings.Join(rows, "\n"))
	}
}

// A group that was running opens when one of its jobs fails, unless the
// user folded it.
func TestJobGroupOpensOnFailure(t *testing.T) {
	f := duckFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab")
	fail := func(names ...string) {
		jobs := slices.Clone(f.jobs[duckRun])
		for i := range jobs {
			if slices.Contains(names, jobs[i].Name) {
				jobs[i].Status, jobs[i].Conclusion = core.RunCompleted, core.ConclusionFailure
			}
		}
		f.setJobs(duckRun, jobs)
		h.run(m.fromCache())
	}
	fail("Test (osx, arm64)")
	if !m.jobs.isOpen("Test (") {
		t.Fatalf("Test failed, but stays folded:\n%s", strings.Join(jobsPaneRows(m), "\n"))
	}
	if j, _ := m.jobs.selected(); j.Name != "Build extension binaries / windows_amd64" {
		t.Errorf("the cursor moved to %q", j.Name)
	}
	// Folded by the user, it stays folded when another of its jobs fails.
	h.keys("j", "j", "enter")
	fail("Test (osx, arm64)", "Test (windows, amd64)")
	if m.jobs.isOpen("Test (") {
		t.Errorf("Test opened again, though the user folded it")
	}
	// A group the user never touched opens on its first failure only.
	fail("Test (osx, arm64)", "Test (windows, amd64)", "Deploy to nightly (linux_amd64)")
	if !m.jobs.isOpen("Deploy to nightly (") {
		t.Errorf("Deploy to nightly failed, but stays folded")
	}
}
