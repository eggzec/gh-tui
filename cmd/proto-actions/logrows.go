package main

import (
	"fmt"
	"strings"
)

var events = []string{"push", "pull_request", "pull_request", "push", "pull_request", "pull_request", "schedule", "push", "pull_request", "pull_request", "schedule", "pull_request", "workflow_dispatch", "push"}

var actors = []string{"ali", "ali", "dependabot", "ali", "ali", "ali", "github-actions", "ali", "bea", "bea", "github-actions", "ali", "bea", "chen"}

// logRow is one line of a raw log: a step marker or a timestamped line.
type logRow struct {
	ts, text, status, dur string
	step                  int
	marker, err           bool
}

// secs is the run's duration in seconds.
func (r run) secs() int {
	var m, s int
	fmt.Sscanf(r.dur, "%dm%ds", &m, &s)
	return m*60 + s
}

// logRows is the job's log: a marker row per step, then its lines.
func (r run) logRows(j job) []logRow {
	var out []logRow
	t := 3
	for si, s := range j.steps {
		out = append(out, logRow{text: s.name, status: s.status, dur: s.dur, step: si, marker: true})
		for _, l := range r.stepLines(j, si) {
			l = strings.ReplaceAll(l, "\t", " ")
			t += 1 + si%3
			ts := fmt.Sprintf("09:%02d:%02d.%03dZ", 10+t/60, t%60, (t*137)%1000)
			out = append(out, logRow{ts: ts, text: l, step: si, err: strings.HasPrefix(l, "##[error]")})
		}
	}
	return out
}
