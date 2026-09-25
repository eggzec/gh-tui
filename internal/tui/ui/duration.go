package ui

import (
	"strconv"
	"time"
)

// Duration writes d the way GitHub shows how long a job took, as the log
// view does: "42s", "1m 5s" or "1h 2m".
func Duration(d time.Duration) string {
	d = max(d, 0).Round(time.Second)
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d/time.Second)) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m " + strconv.Itoa(int(d/time.Second%60)) + "s"
	}
	return strconv.Itoa(int(d/time.Hour)) + "h " + strconv.Itoa(int(d/time.Minute%60)) + "m"
}

// Span is how long something that started at start ran: until end, or
// until now while it runs. It reports false if it hasn't started.
func Span(start, end, now time.Time) (time.Duration, bool) {
	if start.IsZero() {
		return 0, false
	}
	if end.IsZero() || end.Before(start) {
		end = now
	}
	return end.Sub(start), true
}
