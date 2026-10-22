package main

import "fmt"

type step struct{ name, status, dur string }
type job struct {
	name, status, dur string
	steps             []step
}
type run struct {
	id                                      int
	workflow, title, branch, status, dur, age string
	jobs                                    []job
}

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
		out = append(out, run{id: 9100 + 7*i, workflow: s[0], title: s[1], branch: s[2], status: s[3], dur: s[4], age: s[5], jobs: mkJobs(i, s[3])})
	}
	return out
}

func mkJobs(i int, status string) []job {
	names := []string{"lint", "build", "test (linux)", "test (macos)", "e2e", "deploy"}
	base := []string{"Set up job", "Checkout", "Setup Go", "Restore cache", "Run tests", "Build", "Upload artifacts", "Post cleanup"}
	n := 3 + i%4
	var jobs []job
	for ji := 0; ji < n; ji++ {
		k := 4 + (i+ji)%5
		sn := append(append([]string{}, base[:k-1]...), "Post cleanup")
		mid := n / 2
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
		j := job{name: names[ji], status: js}
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
