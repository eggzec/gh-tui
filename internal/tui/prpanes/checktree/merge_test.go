package checktree

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

func mergeOpts() Options {
	o := opts()
	o.Mode = Merge
	return o
}

func TestMergeAllPassing(t *testing.T) {
	t.Parallel()
	c := commit([]core.CommitCheck{
		job(1, 1, "a", core.ConclusionSuccess),
		job(1, 2, "b", core.ConclusionNeutral),
		appRun(codecov, 3, "cov", core.ConclusionSuccess, 0),
	}, status("ci", "success"))
	st := styles(config.IconsASCII)
	if rows, _ := Rows(c, mergeOpts(), st); len(rows) != 0 {
		t.Errorf("rows of passing checks = %v, want none", first(rows))
	}
	if n := ChecksNode(c, mergeOpts(), st); n.ID != "" || n.Fold {
		t.Errorf("node of passing checks = %+v, want none", n)
	}
}

func TestMergeDropsWhatPasses(t *testing.T) {
	t.Parallel()
	rows, _ := Rows(mixed(), mergeOpts(), styles(config.IconsASCII))
	want := []string{
		"c1/run/1", "c1/grp/1/test (", "c1/job/12", "c1/job/14", "c1/grp/1/build /", "c1/job/15", "c1/job/16",
		"c1/app/codecov", "c1/check/21",
		"c1/status/ci/other",
	}
	if got := ids(rows); !slices.Equal(got, want) {
		t.Errorf("ids:\n got %q\nwant %q", got, want)
	}
}

func TestMergeRunCountsJobs(t *testing.T) {
	t.Parallel()
	rows, _ := Rows(mixed(), mergeOpts(), styles(config.IconsASCII))
	if got, want := first(rows)[0], "x > CI #12 | 4 of 6 jobs"; got != want {
		t.Errorf("run row = %q, want %q", got, want)
	}
	if got, want := first(rows)[1], ".x test | 2 of 3 failing"; got != want {
		t.Errorf("matrix row = %q, want %q (counts all of its jobs)", got, want)
	}
}

func TestChecksNode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		c    core.CommitChecks
		want string
	}{
		{"failing", commit([]core.CommitCheck{job(1, 1, "a", core.ConclusionFailure), job(1, 2, "b", core.ConclusionTimedOut)}, status("s", "failure")), "x Checks failing 3"},
		{"failing and running", commit([]core.CommitCheck{job(1, 1, "a", core.ConclusionFailure), running(2, "b")}, status("s", "pending")), "x Checks failing 1 - 2 running"},
		{"running", commit([]core.CommitCheck{job(1, 1, "a", core.ConclusionSuccess), running(2, "b")}), "~ Checks running 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			n := ChecksNode(tt.c, mergeOpts(), styles(config.IconsASCII))
			if got := first([]foldtree.Row{n})[0]; !strings.HasPrefix(got, tt.want) {
				t.Errorf("node = %q, want %q", got, tt.want)
			}
			if !n.Fold || !n.Arrow || n.Closed || n.ID != "c1/checks" {
				t.Errorf("node = %+v, want an open fold with id c1/checks", n)
			}
		})
	}
}

func TestMergeGolden(t *testing.T) {
	t.Parallel()
	st := styles(config.IconsASCII)
	rows, _ := Rows(mixed(), Options{Mode: Merge, Depth: 1, Now: now, Prefix: "c1/"}, st)
	rows = append([]foldtree.Row{ChecksNode(mixed(), mergeOpts(), st)}, rows...)
	golden.RequireEqual(t, text(t, rows, 80, 24))
}
