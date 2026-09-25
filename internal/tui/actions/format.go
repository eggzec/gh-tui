package actions

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// duration writes d the way GitHub shows how long a job took, as the log
// view does: "42s", "1m 5s" or "1h 2m".
func duration(d time.Duration) string {
	d = max(d, 0).Round(time.Second)
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d/time.Second)) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m " + strconv.Itoa(int(d/time.Second%60)) + "s"
	}
	return strconv.Itoa(int(d/time.Hour)) + "h " + strconv.Itoa(int(d/time.Minute%60)) + "m"
}

// span is how long something that started at start ran: until end, or
// until now while it runs. It reports false if it hasn't started.
func span(start, end, now time.Time) (time.Duration, bool) {
	if start.IsZero() {
		return 0, false
	}
	if end.IsZero() || end.Before(start) {
		end = now
	}
	return end.Sub(start), true
}

// runSpan is how long the latest attempt of r ran, or has run so far.
func runSpan(r core.Run, now time.Time) (time.Duration, bool) {
	start := r.RunStartedAt
	if start.IsZero() {
		start = r.CreatedAt
	}
	var end time.Time
	if r.Done() {
		end = r.UpdatedAt
	}
	return span(start, end, now)
}

// stepSpan is how long st ran, or has run so far.
func stepSpan(st core.Step, now time.Time) (time.Duration, bool) {
	if st.Conclusion == core.ConclusionSkipped {
		return 0, false
	}
	var end time.Time
	if st.Status == core.RunCompleted {
		end = st.CompletedAt
	}
	return span(st.StartedAt, end, now)
}

// runName names a run in prose, such as "CI #4812".
func runName(r core.Run) string {
	name := oneLine(r.Name)
	if name == "" {
		name = "Run"
	}
	return name + " #" + strconv.Itoa(r.Number)
}

// failedJobs counts the jobs that a re-run of the failed jobs starts
// again: those that failed or were cancelled.
func failedJobs(jobs []core.Job) int {
	n := 0
	for i := range jobs {
		if c := jobs[i].Conclusion; c.Failed() || c == core.ConclusionCancelled {
			n++
		}
	}
	return n
}

// firstLine cuts s at its first line.
func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

// oneLine joins the lines of s, and drops the control characters that
// would break a row.
func oneLine(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	return strings.Join(strings.FieldsFunc(s, isControl), " ")
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0
}

// fit pads or cuts s to w cells.
func fit(s string, w int) string {
	sw := ansi.StringWidth(s)
	switch {
	case sw == w:
		return s
	case sw < w:
		return s + strings.Repeat(" ", w-sw)
	}
	return ansi.Truncate(s, w, "")
}

// padLines returns exactly h of lines, which are w cells wide already:
// the lines past h are left out, and blank ones fill the rest.
func padLines(lines []string, w, h int) []string {
	h = max(h, 0)
	if len(lines) >= h {
		return lines[:h]
	}
	blank := strings.Repeat(" ", w)
	for len(lines) < h {
		lines = append(lines, blank)
	}
	return lines
}

// fitLines returns exactly h lines of exactly w cells: lines cut or padded.
func fitLines(lines []string, w, h int) []string {
	out := make([]string, max(h, 0))
	for i := range out {
		if i < len(lines) {
			out[i] = fit(lines[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// wrap wraps s to lines of w cells, each after indent.
func wrap(s string, w int, indent string) []string {
	iw := ansi.StringWidth(indent)
	lines := strings.Split(ansi.Wrap(s, max(w-iw, 1), ""), "\n")
	for i, l := range lines {
		lines[i] = fit(indent+l, w)
	}
	return lines
}

// spread puts left and right on a line of w cells, with right against the
// edge. Left is cut to leave right whole, while right fits.
func spread(left, right string, w int) string {
	rw := ansi.StringWidth(right)
	if rw == 0 {
		return fit(left, w)
	}
	if rw+2 > w {
		return fit(left, w)
	}
	room := w - rw - 1
	left = ansi.Truncate(left, room, "…")
	return left + strings.Repeat(" ", w-ansi.StringWidth(left)-rw) + right
}
