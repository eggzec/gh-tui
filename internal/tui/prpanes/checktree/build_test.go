package checktree

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// mixed is a commit with every kind of check.
func mixed() core.CommitChecks {
	c := commit([]core.CommitCheck{
		job(1, 11, "lint", core.ConclusionSuccess),
		job(1, 12, "test (linux)", core.ConclusionFailure),
		job(1, 13, "test (osx)", core.ConclusionSuccess),
		job(1, 14, "test (windows)", core.ConclusionCancelled),
		running(15, "build / docs"),
		running(16, "build / site"),
		appRun(codecov, 21, "coverage", core.ConclusionFailure, 3),
		appRun(codecov, 22, "patch", core.ConclusionSuccess, 0),
		appRun(nil, 23, "legacy", core.ConclusionNeutral, 0),
		job(2, 31, "deploy", core.ConclusionSuccess),
	}, status("ci/other", "pending"), status("ci/old", "success"))
	c.Runs[9].Workflow = "Deploy"
	c.Runs[9].RunNumber = 7
	c.Runs[9].App = &core.CheckApp{Name: "GitHub Actions", Slug: "github-actions"}
	return c
}

func TestRowsOrder(t *testing.T) {
	t.Parallel()
	rows, _ := Rows(mixed(), opts(), styles(config.IconsASCII))
	want := []string{
		"c1/run/1", "c1/job/11", "c1/grp/1/test (", "c1/job/12", "c1/job/13", "c1/job/14", "c1/grp/1/build /", "c1/job/15", "c1/job/16",
		"c1/run/2", "c1/job/31",
		"c1/app/codecov", "c1/check/21", "c1/check/22",
		"c1/other", "c1/check/23",
		"c1/status/ci/other", "c1/status/ci/old",
	}
	if got := ids(rows); !slices.Equal(got, want) {
		t.Errorf("ids:\n got %q\nwant %q", got, want)
	}
}

func TestRowsRightParts(t *testing.T) {
	t.Parallel()
	rows, _ := Rows(mixed(), opts(), styles(config.IconsASCII))
	byID := map[string]string{}
	for _, r := range rows {
		byID[r.ID] = strings.TrimLeft(first([]foldtree.Row{r})[0], ".")
	}
	for id, want := range map[string]string{
		"c1/run/1":         "x > CI #12 | failed",
		"c1/run/2":         "+ > Deploy #7 | passed",
		"c1/grp/1/test (":  "x test | 2 of 3 failing",
		"c1/grp/1/build /": "~ build | 2 of 2 running",
		"c1/job/12":        "x linux | failed - 1m 5s",
		"c1/job/13":        "+ osx | passed - 1m 5s",
		"c1/job/15":        "~ docs | running - 12s",
		"c1/app/codecov":   "x # Codecov | failed",
		"c1/check/21":      "x coverage | 3 findings - failed",
		"c1/check/22":      "+ patch | passed - 30s",
		"c1/other":         "+ Other checks | passed",
		"c1/status/ci/old": "+ . ci/old | ci/old success",
	} {
		if byID[id] != want {
			t.Errorf("%s:\n got %q\nwant %q", id, byID[id], want)
		}
	}
}

func TestGroupsEqualCoreGroupJobs(t *testing.T) {
	t.Parallel()
	names := []string{"build / linux", "lint", "build / osx", "test (a)", "test (b)", "vars"}
	runs := make([]core.CommitCheck, 0, len(names))
	for i, n := range names {
		runs = append(runs, job(1, int64(i+1), n, core.ConclusionSuccess))
	}
	rows, _ := Rows(commit(runs), opts(), styles(config.IconsASCII))
	var folds, leaves int
	for _, r := range rows[1:] {
		if r.Fold {
			folds++
		} else {
			leaves++
		}
	}
	if folds != 2 || leaves != 6 {
		t.Errorf("%d groups and %d jobs, want 2 and 6 (a caller and a matrix, and the six jobs):\n%s", folds, leaves, strings.Join(first(rows), "\n"))
	}
}

func TestStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		runs []core.CommitCheck
		sts  []core.StatusContext
		want core.CheckState
	}{
		{"nothing", nil, nil, core.CheckNone},
		{"neutral and skipped pass", []core.CommitCheck{job(1, 1, "a", core.ConclusionNeutral), job(1, 2, "b", core.ConclusionSkipped)}, nil, core.CheckPassing},
		{"cancelled fails", []core.CommitCheck{job(1, 1, "a", core.ConclusionSuccess), job(1, 2, "b", core.ConclusionCancelled)}, nil, core.CheckFailing},
		{"a pending status beats passing runs", []core.CommitCheck{job(1, 1, "a", core.ConclusionSuccess)}, []core.StatusContext{status("x", "pending")}, core.CheckPending},
		{"failing beats pending", []core.CommitCheck{running(1, "a")}, []core.StatusContext{status("x", "error")}, core.CheckFailing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := StateOf(commit(tt.runs, tt.sts...)); got != tt.want {
				t.Errorf("StateOf = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCommitIcon(t *testing.T) {
	t.Parallel()
	st := styles(config.IconsASCII)
	if got := CommitIcon(core.ChecksNone, st); got != "" {
		t.Errorf("icon of no checks = %q, want none", got)
	}
	for s, want := range map[core.ChecksState]string{core.ChecksFailure: "x", core.ChecksPending: "~", core.ChecksSuccess: "+"} {
		if got := text1(CommitIcon(s, st)); got != want {
			t.Errorf("icon of %v = %q, want %q", s, got, want)
		}
	}
}

func TestRowsNoChecks(t *testing.T) {
	t.Parallel()
	if rows, _ := Rows(core.CommitChecks{SHA: sha}, opts(), styles(config.IconsASCII)); rows != nil {
		t.Errorf("rows of no checks = %v, want none", rows)
	}
}

func TestFindingsSingular(t *testing.T) {
	t.Parallel()
	c := commit([]core.CommitCheck{appRun(codecov, 1, "cov", core.ConclusionFailure, 1)})
	c.Runs[0].Notes = []core.Annotation{{Level: core.AnnotationWarning}}
	rows, _ := Rows(c, opts(), styles(config.IconsASCII))
	if got := first(rows)[1]; !strings.Contains(got, "! 1 finding - failed") {
		t.Errorf("row = %q, want a warning glyph and %q", got, "1 finding - failed")
	}
	c.Runs[0].Notes = nil
	rows, _ = Rows(c, opts(), styles(config.IconsASCII))
	if got := first(rows)[1]; !strings.HasSuffix(got, "| 1 finding - failed") {
		t.Errorf("row without notes = %q, want no glyph before the count", got)
	}
}

func TestTargets(t *testing.T) {
	t.Parallel()
	rows, targets := Rows(mixed(), opts(), styles(config.IconsASCII))
	if got, want := targets["c1/job/12"], (OpenJobMsg{SHA: sha, RunID: 1, JobID: 12}); got != want {
		t.Errorf("job target = %v, want %v", got, want)
	}
	if got, want := targets["c1/check/21"], (OpenCheckMsg{SHA: sha, CheckID: 21, SuiteID: 77}); got != want {
		t.Errorf("check target = %v, want %v", got, want)
	}
	for _, r := range rows {
		_, has := targets[r.ID]
		if has != r.Action {
			t.Errorf("%s: target %v but Action %v", r.ID, has, r.Action)
		}
		if strings.HasPrefix(r.ID, "c1/status/") && (has || r.Action) {
			t.Errorf("status row %s must do nothing on enter", r.ID)
		}
	}
}

func TestIDs(t *testing.T) {
	t.Parallel()
	a, _ := Rows(mixed(), Options{Now: now, Prefix: "a/"}, styles(config.IconsASCII))
	b, _ := Rows(mixed(), Options{Now: now, Prefix: "b/"}, styles(config.IconsASCII))
	again, _ := Rows(mixed(), Options{Now: now, Prefix: "a/"}, styles(config.IconsASCII))
	seen := map[string]bool{}
	for _, r := range append(slices.Clone(a), b...) {
		if seen[r.ID] {
			t.Errorf("ID %q appears twice across two commits", r.ID)
		}
		seen[r.ID] = true
	}
	if !slices.Equal(ids(a), ids(again)) {
		t.Errorf("IDs changed between two calls:\n%q\n%q", ids(a), ids(again))
	}
	for _, r := range a {
		if r.Closed {
			t.Errorf("%s starts closed", r.ID)
		}
	}
}

func TestTruncated(t *testing.T) {
	t.Parallel()
	c := commit([]core.CommitCheck{job(1, 1, "a", core.ConclusionSuccess)})
	c.Truncated, c.Total = true, 41
	rows, _ := Rows(c, opts(), styles(config.IconsASCII))
	last := rows[len(rows)-1]
	if !last.Skip || ansiStrip(last.Lines[0]) != "40 more checks not shown" {
		t.Errorf("last row = %+v, want a skip row of 40 more checks", last)
	}
}

func TestRowsShareNothing(t *testing.T) {
	t.Parallel()
	c := mixed()
	Rows(c, opts(), styles(config.IconsASCII))
	if want := mixed(); !slices.EqualFunc(c.Runs, want.Runs, func(a, b core.CommitCheck) bool { return a.ID == b.ID && a.Name == b.Name }) {
		t.Error("Rows changed the runs it was given")
	}
}

func TestRowsCleanHostileText(t *testing.T) {
	t.Parallel()
	h := termtexttest.Hostile
	c := mixed()
	for i := range c.Runs {
		c.Runs[i].Name, c.Runs[i].Workflow = h, h
		c.Runs[i].DetailsURL = "https://github.com/o/r/" + "x"
	}
	c.Runs[6].App = &core.CheckApp{Name: h, Slug: "codecov"}
	c.Statuses[0].Context, c.Statuses[0].Description = h, h
	for _, width := range []int{120, 40} {
		rows, _ := Rows(c, opts(), styles(config.IconsASCII))
		termtexttest.AssertClean(t, drawn(rows, width, 30), width)
	}
}

func TestGoldens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, set     string
		width, height int
	}{
		{"nerd 120", config.IconsNerd, 120, 24},
		{"ascii 80", config.IconsASCII, 80, 24},
		{"unicode 40", config.IconsUnicode, 40, 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rows, _ := Rows(mixed(), opts(), styles(tt.set))
			golden.RequireEqual(t, text(t, rows, tt.width, tt.height))
		})
	}
}

func BenchmarkRows(b *testing.B) {
	runs := make([]core.CommitCheck, 0, 200)
	for i := range 200 {
		runs = append(runs, job(int64(i/20+1), int64(i+1), "job"+string(rune('a'+i%26))+" (x"+string(rune('a'+i%20))+")", core.ConclusionSuccess))
	}
	c := commit(runs)
	st := styles(config.IconsNerd)
	b.ReportAllocs()
	for b.Loop() {
		Rows(c, opts(), st)
	}
}
