package calendar

import (
	"slices"
	"strconv"
	"time"
)

const (
	// cellW is the width of a week column: the day and the gap after it.
	// The last column needs no gap, so n weeks take cellW*n-1 cells.
	cellW = 2
	// gutterW is the width of the weekday labels and the space after them.
	gutterW = 4
	// minLabeledWeeks is the fewest weeks worth keeping the weekday labels
	// for; below it the labels make room for more weeks.
	minLabeledWeeks = 8
)

// Lines of the view, top to bottom.
const (
	lineTotal = iota
	lineMonths
	lineDays
	lineFooter   = lineDays + 7
	contentLines = lineFooter + 1
)

// slot is a place in the grid, which holds a day unless the data has none
// for it, as before the first day of a partial first week.
type slot struct {
	day Day
	ok  bool
}

// week is a column of the grid, indexed by weekday from Sunday.
type week [7]slot

// first returns the first day of w.
func (w *week) first() Day {
	for _, s := range w {
		if s.ok {
			return s.day
		}
	}
	return Day{}
}

// fitWeeks returns how many week columns fit in width cells.
func fitWeeks(width int) int {
	return max((width+1)/cellW, 0)
}

// showWeekdays reports whether a calendar width cells wide keeps its
// weekday labels.
func showWeekdays(width int) bool {
	return width >= gutterW+cellW*minLabeledWeeks-1
}

// monthLabel is the name of a month above the column it starts at.
type monthLabel struct {
	col  int
	name string
}

// monthLabels places a label above the first week of each month in weeks,
// where the month of a week is the month of its first day, as on GitHub.
// Labels are kept only if they fit in room cells and leave a space before
// the next one. When two collide the later one wins, since the earlier is a
// month that has only begun at the left edge.
func monthLabels(weeks []week, room int) []monthLabel {
	var all []monthLabel
	var prev time.Month
	for i := range weeks {
		d := weeks[i].first()
		if m := d.Date.Month(); m != prev {
			all = append(all, monthLabel{col: i, name: d.Date.Format("Jan")})
			prev = m
		}
	}
	kept := make([]monthLabel, 0, len(all))
	next := room + 1
	for _, l := range slices.Backward(all) {
		x := cellW * l.col
		if end := x + len(l.name); end < next && end <= room {
			kept = append(kept, l)
			next = x
		}
	}
	slices.Reverse(kept)
	return kept
}

// commas formats n with a comma between each group of three digits.
func commas(n int) string {
	s := strconv.Itoa(n)
	neg := n < 0
	if neg {
		s = s[1:]
	}
	b := make([]byte, 0, len(s)+len(s)/3+1)
	if neg {
		b = append(b, '-')
	}
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			b = append(b, ',')
		}
		b = append(b, s[i])
	}
	return string(b)
}

// contributions returns "1 contribution", "12 contributions" and so on, and
// none for zero.
func contributions(n int, none string) string {
	switch n {
	case 0:
		return none
	case 1:
		return "1 contribution"
	default:
		return commas(n) + " contributions"
	}
}

// totalText is the line above the grid.
func totalText(total int) string {
	return contributions(total, "0 contributions") + " in the last year"
}

// statusText is the line about the day under the cursor.
func statusText(d Day) string {
	return contributions(d.Count, "No contributions") + " on " + d.Date.Format("Mon, Jan 2")
}
