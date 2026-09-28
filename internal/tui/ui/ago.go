package ui

import (
	"strconv"
	"time"
)

// Ago is how long before now t was, in the short form lists use, such as
// "5m", "3h" or "2d". Times in the future read as "now".
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	case d < 365*24*time.Hour:
		return strconv.Itoa(int(d/(30*24*time.Hour))) + "mo"
	}
	return strconv.Itoa(int(d/(365*24*time.Hour))) + "y"
}

// AgoProse is Ago as prose, such as "3d ago" or "just now", for sentences
// like "updated 3d ago".
func AgoProse(t, now time.Time) string {
	a := Ago(t, now)
	if a == "now" {
		return "just now"
	}
	return a + " ago"
}

// Clock is the time of day of t in its zone, such as "15:04", for times
// near enough that an age would be too coarse, such as when a limit lifts.
// It adds the day when that isn't the day of now, "Jan 2 15:04", and the
// year when that isn't the year of now, "Jan 2 2027 15:04".
func Clock(t, now time.Time) string {
	now = now.In(t.Location())
	switch {
	case t.Year() != now.Year():
		return t.Format("Jan 2 2006 15:04")
	case t.YearDay() != now.YearDay():
		return t.Format("Jan 2 15:04")
	}
	return t.Format("15:04")
}
