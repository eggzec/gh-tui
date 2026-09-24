package calendar_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
)

func Example() {
	// The tui adapts a user's contribution calendar from the API to weeks
	// of days, from Sunday; here it is three weeks, the first one partial.
	start := time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC)
	var weeks [][]calendar.Day
	for i := range 17 {
		d := start.AddDate(0, 0, i)
		if len(weeks) == 0 || d.Weekday() == time.Sunday {
			weeks = append(weeks, nil)
		}
		weeks[len(weeks)-1] = append(weeks[len(weeks)-1], calendar.Day{Date: d, Count: i % 5, Level: i % 5})
	}

	m := calendar.New(
		calendar.WithWeeks(weeks),
		calendar.WithTotal(1234),
		calendar.WithSize(40, 10),
	)
	// Focus it to move a cursor over the days; a calendar.SelectMsg whose
	// ID is m.ID() names the day under it.
	for line := range strings.Lines(ansi.Strip(m.View())) {
		fmt.Println(strings.TrimRight(line, " \n"))
	}
	// Output:
	// 1,234 contributions in the last year
	//     Sep
	//       ■ ■
	// Mon   ■ ■
	//       ■ ■
	// Wed ■ ■ ■
	//     ■ ■ ■
	// Fri ■ ■ ■
	//     ■ ■
	//                      Less ■ ■ ■ ■ ■ More
}
