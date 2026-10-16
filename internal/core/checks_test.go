package core

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckStateOf(t *testing.T) {
	tests := []struct {
		value string
		want  CheckState
	}{
		{"failure", CheckFailing},
		{"timed_out", CheckFailing},
		{"startup_failure", CheckFailing},
		{"action_required", CheckFailing},
		{"cancelled", CheckFailing},
		{"stale", CheckFailing},
		{"error", CheckFailing},
		{"queued", CheckPending},
		{"in_progress", CheckPending},
		{"waiting", CheckPending},
		{"requested", CheckPending},
		{"pending", CheckPending},
		{"expected", CheckPending},
		{"success", CheckPassing},
		{"neutral", CheckPassing},
		{"skipped", CheckPassing},
		{"", CheckPending},
		{"something_new", CheckPending},
		{"SUCCESS", CheckPending},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := CheckStateOf(tt.value); got != tt.want {
				t.Errorf("CheckStateOf(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestWorst(t *testing.T) {
	tests := []struct {
		name   string
		states []CheckState
		want   CheckState
	}{
		{"no checks", nil, CheckNone},
		{"only none", []CheckState{CheckNone, CheckNone}, CheckNone},
		{"passing", []CheckState{CheckPassing, CheckPassing}, CheckPassing},
		{"pending over passing", []CheckState{CheckPassing, CheckPending, CheckPassing}, CheckPending},
		{"failing over pending", []CheckState{CheckPending, CheckFailing, CheckPassing}, CheckFailing},
		{"failing last", []CheckState{CheckPassing, CheckFailing}, CheckFailing},
		{"passing over none", []CheckState{CheckNone, CheckPassing}, CheckPassing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Worst(tt.states...); got != tt.want {
				t.Errorf("Worst = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOutcome(t *testing.T) {
	tests := []struct {
		name   string
		status RunStatus
		c      Conclusion
		want   CheckState
	}{
		{"queued", RunQueued, ConclusionNone, CheckPending},
		{"running", RunInProgress, ConclusionNone, CheckPending},
		{"waiting", RunWaiting, ConclusionNone, CheckPending},
		{"requested", RunRequested, ConclusionNone, CheckPending},
		{"pending", RunPending, ConclusionNone, CheckPending},
		{"not completed whatever it says", RunInProgress, ConclusionFailure, CheckPending},
		{"success", RunCompleted, ConclusionSuccess, CheckPassing},
		{"neutral", RunCompleted, ConclusionNeutral, CheckPassing},
		{"skipped", RunCompleted, ConclusionSkipped, CheckPassing},
		{"failure", RunCompleted, ConclusionFailure, CheckFailing},
		{"timed out", RunCompleted, ConclusionTimedOut, CheckFailing},
		{"startup failure", RunCompleted, ConclusionStartupFailure, CheckFailing},
		{"action required", RunCompleted, ConclusionActionRequired, CheckFailing},
		{"cancelled", RunCompleted, ConclusionCancelled, CheckFailing},
		{"completed without a conclusion", RunCompleted, ConclusionNone, CheckPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Check{Status: tt.status, Conclusion: tt.c}).Outcome(); got != tt.want {
				t.Errorf("check: %d, want %d", got, tt.want)
			}
			if got := (Job{Status: tt.status, Conclusion: tt.c}).Outcome(); got != tt.want {
				t.Errorf("job: %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStatusOutcome(t *testing.T) {
	tests := []struct {
		state string
		want  CheckState
	}{
		{"error", CheckFailing},
		{"failure", CheckFailing},
		{"success", CheckPassing},
		{"pending", CheckPending},
		{"expected", CheckPending},
		{"unheard of", CheckPending},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := (StatusContext{State: tt.state}).Outcome(); got != tt.want {
				t.Errorf("state %q: %d, want %d", tt.state, got, tt.want)
			}
		})
	}
}

// shape writes nodes as "job" and "group{kid | kid}", to compare trees.
func shape(nodes []JobNode) string {
	parts := make([]string, len(nodes))
	for i, n := range nodes {
		parts[i] = n.Label
		if n.Group != nil {
			parts[i] += "{" + shape(n.Group.Kids) + "}"
		}
	}
	return strings.Join(parts, " | ")
}

func named(names ...string) []Job {
	jobs := make([]Job, len(names))
	for i, n := range names {
		jobs[i] = Job{ID: int64(i + 1), Name: n, Status: RunCompleted, Conclusion: ConclusionSuccess}
	}
	return jobs
}

func TestGroupJobs(t *testing.T) {
	tests := []struct {
		name  string
		jobs  []string
		shape string
	}{
		{"no jobs", nil, ""},
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
			"a name on several lines",
			[]string{"build\n(a)", "build\x1b[31m (b)"},
			"build{a | b}",
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
			if got := shape(GroupJobs(named(tt.jobs...))); got != tt.shape {
				t.Errorf("groups\n %s\nwant\n %s", got, tt.shape)
			}
		})
	}
}

// A group lists the jobs of its kids too, and takes the worst of their
// states.
func TestGroupJobsRollup(t *testing.T) {
	done := func(name string, c Conclusion) Job {
		return Job{Name: name, Status: RunCompleted, Conclusion: c}
	}
	jobs := []Job{
		done("build / linux (a)", ConclusionSuccess),
		done("build / linux (b)", ConclusionSkipped),
		{Name: "build / osx", Status: RunInProgress},
		done("lint", ConclusionFailure),
		done("test (x)", ConclusionSuccess),
		done("test (y)", ConclusionSuccess),
	}
	nodes := GroupJobs(jobs)
	if got, want := shape(nodes), "build{linux{a | b} | osx} | lint | test{x | y}"; got != want {
		t.Fatalf("groups\n %s\nwant\n %s", got, want)
	}
	build, test := nodes[0].Group, nodes[2].Group
	if want := []int{0, 1, 2}; fmt.Sprint(build.Jobs) != fmt.Sprint(want) || build.State != CheckPending {
		t.Errorf("build: jobs %v state %d, want %v and pending", build.Jobs, build.State, want)
	}
	if linux := build.Kids[0].Group; linux.State != CheckPassing || fmt.Sprint(linux.Jobs) != "[0 1]" {
		t.Errorf("linux: jobs %v state %d, want [0 1] and passing", linux.Jobs, linux.State)
	}
	if test.State != CheckPassing {
		t.Errorf("test state %d, want passing", test.State)
	}
	if nodes[1].Job != 3 || nodes[1].Group != nil {
		t.Errorf("lint is job %d with group %v, want job 3 alone", nodes[1].Job, nodes[1].Group)
	}
}
