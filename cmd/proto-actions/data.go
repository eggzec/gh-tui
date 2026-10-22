package main

import (
	"fmt"
	"strings"
)

type step struct{ name, status, dur string }
type job struct {
	name, status, dur string
	group             string   // matrix parent name, "" when not a matrix job
	needs             []string // names of the jobs (or matrix parents) it waits for
	steps             []step
}

type annotation struct {
	level, file, msg string
	line, step       int // step: index of the step to jump to
}

type artifact struct{ name, size, expiry string }

// annotations are hardcoded for the failing job.
func (r run) annotations(j job) []annotation {
	if j.status != "failure" {
		return nil
	}
	_, fs := r.failing()
	return []annotation{
		{"error", "retry_test.go", "expected 3 attempts, got 2", 88, fs},
		{"error", "retry_test.go", "--- FAIL: TestRetry (0.31s)", 74, fs},
		{"error", ".github/workflows/ci.yml", "Process completed with exit code 1.", 41, fs},
		{"warning", "internal/tui/actions/log.go", "cache miss for restore key go-mod-linux", 12, max(0, fs-1)},
		{"warning", "go.mod", "toolchain go1.24.1 is newer than go directive", 3, 2},
		{"notice", "Makefile", "retrying flaky target test-race", 57, fs},
	}
}

func (r run) artifacts() []artifact {
	switch r.status {
	case "failure":
		return []artifact{{"test-results.xml", "48 KB", "in 29d"}, {"coverage.out", "212 KB", "in 29d"}, {"gh-tui-linux-amd64", "14.2 MB", "in 6d"}}
	case "success":
		return []artifact{{"gh-tui-linux-amd64", "14.2 MB", "in 6d"}, {"coverage.out", "212 KB", "in 29d"}}
	}
	return nil
}

// stepLines is what one step printed, shown under its header.
func (r run) stepLines(j job, si int) []string {
	s := j.steps[si]
	pool := []string{"go: downloading charm.land/lipgloss/v2 v2.0.6", "cache hit for key go-mod-linux", "Compiling package internal/tui", "Running 214 tests", "ok  internal/tui/actions 0.412s", "Linking binary", "Uploading 3 files"}
	n := 2 + (r.id+si)%3
	var out []string
	if s.status == "skipped" || s.status == "queued" {
		return nil
	}
	for i := range n {
		out = append(out, pool[(r.id+si*3+i)%len(pool)])
	}
	if s.status == "failure" {
		out = append(out, "##[error]--- FAIL: TestRetry (0.31s)", "##[error]    retry_test.go:88: expected 3 attempts, got 2", "##[error]Process completed with exit code 1.")
	}
	if si == 3 {
		out = append(out, "##[warning]cache miss for restore key")
	}
	return out
}

type run struct {
	id                                        int
	event, actor                              string
	workflow, title, branch, status, dur, age string
	jobs                                      []job
}

// sha is a short commit id made from the run's id.
func (r run) sha() string { return fmt.Sprintf("%07x", r.id*2654435%0xfffffff) }

var workflows = []string{"CI", "Release", "Nightly"}

var allRuns = buildRuns()

func buildRuns() []run {
	spec := [][]string{
		{"CI", "Fix flaky retry test", "fix/retry", "failure", "3m12s", "4m"},
		{"CI", "Add owner page panes", "feat/owner", "running", "1m40s", "2m"},
		{"CI", "Bump bubbletea to 2.0.9", "deps/bt", "success", "4m02s", "18m"},
		{"Release", "v0.14.0", "main", "success", "9m31s", "1h"},
		{"CI", "Refactor ownerui frame", "refactor/ui", "success", "3m48s", "2h"},
		{"CI", "Tab-complete search qualifiers", "feat/search", "queued", "", "10s"},
		{"Nightly", "Nightly e2e", "main", "failure", "21m07s", "5h"},
		{"Release", "v0.14.0-rc1", "main", "cancelled", "2m11s", "1d"},
		{"CI", "Docs: keybinding table", "docs/keys", "skipped", "", "1d"},
		{"CI", "Fix log pager wrap", "fix/pager", "success", "3m55s", "1d"},
		{"Nightly", "Nightly e2e", "main", "success", "19m50s", "1d"},
		{"CI", "Draft: actions redesign", "proto/actions", "running", "0m52s", "1m"},
		{"Release", "Publish docs", "main", "success", "1m33s", "2d"},
		{"CI", "Cache module downloads", "ci/cache", "cancelled", "0m44s", "2d"},
	}
	var out []run
	for i, s := range spec {
		out = append(out, run{id: 9100 + 7*i, event: events[i], actor: actors[i], workflow: s[0], title: s[1], branch: s[2], status: s[3], dur: s[4], age: s[5], jobs: mkJobs(i, s[3])})
	}
	return out
}

func mkJobs(i int, status string) []job {
	// three shapes: a chain, a fan-out/fan-in with a matrix, and an 11-job pipeline
	t := []string{"test (go 1.23, ubuntu)", "test (go 1.24, ubuntu)", "test (go 1.23, macos)", "test (go 1.24, macos)"}
	names := append(append([]string{"lint", "build"}, t...), "package", "release")
	needs := [][]string{nil, nil, {"build", "lint"}, {"build", "lint"}, {"build", "lint"}, {"build", "lint"}, {"test"}, {"package"}}
	switch {
	case i == 3 || i == 7 || i == 12:
		names, needs = []string{"build", "test", "deploy"}, [][]string{nil, {"build"}, {"test"}}
	case i == 6 || i == 10:
		names = append(append([]string{"setup", "lint", "vet", "build"}, t...), "e2e", "package", "release")
		needs = [][]string{nil, {"setup"}, {"setup"}, {"lint", "vet"}, {"build"}, {"build"}, {"build"}, {"build"}, {"test"}, {"e2e"}, {"package"}}
	}
	base := []string{"Set up job", "Checkout", "Setup Go", "Restore cache", "Run tests", "Build", "Upload artifacts", "Post cleanup"}
	n := len(names)
	var jobs []job
	for ji := 0; ji < n; ji++ {
		k := 4 + (i+ji)%5
		sn := append(append([]string{}, base[:k-1]...), "Post cleanup")
		mid := 3
		js := "success"
		ss := func(si int) string { return "success" }
		switch status {
		case "failure":
			fs := min(4, k-2)
			if ji == mid {
				js = "failure"
				ss = func(si int) string {
					switch {
					case si < fs:
						return "success"
					case si == fs:
						return "failure"
					case si == k-1:
						return "success"
					}
					return "skipped"
				}
			} else if ji > mid {
				js = "skipped"
				ss = func(int) string { return "skipped" }
			}
		case "running":
			rs := k / 2
			if ji == mid {
				js = "running"
				ss = func(si int) string {
					switch {
					case si < rs:
						return "success"
					case si == rs:
						return "running"
					}
					return "queued"
				}
			} else if ji > mid {
				js = "queued"
				ss = func(int) string { return "queued" }
			}
		case "cancelled":
			cs := k / 2
			if ji == mid {
				js = "cancelled"
				ss = func(si int) string {
					switch {
					case si < cs:
						return "success"
					case si == cs:
						return "cancelled"
					}
					return "skipped"
				}
			} else if ji > mid {
				js = "cancelled"
				ss = func(int) string { return "skipped" }
			}
		case "queued", "skipped":
			js = status
			ss = func(int) string { return status }
		}
		j := job{name: names[ji], status: js, needs: needs[ji]}
		j.group, _, _ = strings.Cut(j.name, " (")
		if j.group == j.name {
			j.group = ""
		}
		if js != "queued" && js != "skipped" {
			j.dur = fmt.Sprintf("%ds", 10+7*(i+ji)%50)
		}
		for si, name := range sn {
			st := step{name: name, status: ss(si)}
			if st.status != "queued" && st.status != "skipped" {
				st.dur = fmt.Sprintf("%ds", 1+(i*ji+si*3)%20)
			}
			j.steps = append(j.steps, st)
		}
		jobs = append(jobs, j)
	}
	return jobs
}

// failing returns the failing job index and step index, or -1.
func (r run) failing() (int, int) {
	for ji, j := range r.jobs {
		for si, s := range j.steps {
			if s.status == "failure" {
				return ji, si
			}
		}
	}
	return -1, -1
}

func (r run) doneJobs() int {
	n := 0
	for _, j := range r.jobs {
		if j.status == "success" {
			n++
		}
	}
	return n
}

func logFor(r run, j job) []string {
	msgs := []string{
		"Downloading dependencies", "go: downloading charm.land/lipgloss/v2 v2.0.6", "Compiling package internal/tui",
		"cache hit for key go-mod-linux", "Running 214 tests", "ok  \tinternal/tui/actions\t0.412s",
		"##[warning]cache miss for restore key", "Linking binary", "Uploading 3 files", "Verifying checksums",
	}
	_, fs := r.failing()
	_ = fs
	out := make([]string, 60)
	for i := range out {
		ts := fmt.Sprintf("09:%02d:%02d", 10+i/30, (i*2)%60)
		switch {
		case i%12 == 0:
			out[i] = ts + " ##[group]" + j.steps[(i/12)%len(j.steps)].name
		case j.status == "failure" && i == 52:
			out[i] = ts + " ##[error]--- FAIL: TestRetry (0.31s)"
		case j.status == "failure" && i == 53:
			out[i] = ts + " ##[error]    retry_test.go:88: expected 3 attempts, got 2"
		case j.status == "failure" && i == 54:
			out[i] = ts + " ##[error]Process completed with exit code 1."
		default:
			out[i] = ts + " " + msgs[(i*7+r.id)%len(msgs)]
		}
	}
	return out
}
