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
