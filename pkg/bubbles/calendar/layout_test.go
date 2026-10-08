package calendar

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// weeksFrom returns n full weeks from the Sunday start.
func weeksFrom(start time.Time, n int) []week {
	weeks := make([]week, n)
	for i := range n * 7 {
		weeks[i/7][i%7] = slot{day: Day{Date: start.AddDate(0, 0, i)}, ok: true}
	}
	return weeks
}

func TestMonthLabels(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		weeks int
		room  int
		want  []monthLabel
	}{
		{
			// Sep 1 2026 is a Tuesday, so its week is August's.
			name: "at the first week that starts in the month", start: date(2026, 8, 2), weeks: 9, room: 100,
			want: []monthLabel{{0, "Aug"}, {5, "Sep"}},
		},
		{
			name: "a month that has only begun gives way", start: date(2026, 8, 30), weeks: 4, room: 100,
			want: []monthLabel{{1, "Sep"}},
		},
		{
			name: "two weeks leave room for both", start: date(2026, 8, 23), weeks: 6, room: 100,
			want: []monthLabel{{0, "Aug"}, {2, "Sep"}},
		},
		{
			name: "a label past the right edge is dropped", start: date(2026, 8, 2), weeks: 6, room: 12,
			want: []monthLabel{{0, "Aug"}},
		},
		{
			name: "a label that just fits stays", start: date(2026, 8, 2), weeks: 6, room: 13,
			want: []monthLabel{{0, "Aug"}, {5, "Sep"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monthLabels(weeksFrom(tt.start, tt.weeks), tt.room)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("labels %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMonthLabelsPartialFirstWeek(t *testing.T) {
	// The partial first week starts on Wednesday Sep 24 2025, so it is
	// September's, and October's label goes on the week of Sunday Oct 5.
	m := newModel(WithWeeks(year(today)), WithSize(120, 10))
	labels := monthLabels(m.grid[m.start:m.start+m.cols], 120)
	if labels[0] != (monthLabel{0, "Sep"}) || labels[1] != (monthLabel{2, "Oct"}) {
		t.Fatalf("labels start %v, want Sep at 0 and Oct at 2", labels[:2])
	}
	if len(labels) != 13 {
		t.Fatalf("%d labels, want 13: %v", len(labels), labels)
	}
	line := ansi.Strip(m.lines[lineMonths])
	if !strings.HasPrefix(line, "    Sep Oct ") {
		t.Fatalf("month line %q", line)
	}
}

func TestLayout(t *testing.T) {
	tests := []struct {
		width    int
		weekdays bool
		cols     int
		legend   bool
	}{
		{120, true, 53, true},
		{110, true, 53, true},
		{80, true, 38, true},
		{40, true, 18, true},
		{19, true, 8, true},
		{18, false, 9, false},
		{1, false, 1, false},
	}
	for _, tt := range tests {
		m := newModel(WithWeeks(year(today)), WithSize(tt.width, 10))
		if m.weekdays != tt.weekdays || m.cols != tt.cols {
			t.Errorf("width %d: weekdays %v and %d weeks, want %v and %d", tt.width, m.weekdays, m.cols, tt.weekdays, tt.cols)
		}
		// The weeks shown are the most recent ones.
		if m.start+m.cols != 53 {
			t.Errorf("width %d: weeks %d to %d shown, want the last %d", tt.width, m.start, m.start+m.cols, tt.cols)
		}
		if got := strings.Contains(ansi.Strip(m.lines[lineFooter]), "Less"); got != tt.legend {
			t.Errorf("width %d: legend shown %v, want %v", tt.width, got, tt.legend)
		}
		if got := strings.Contains(ansi.Strip(m.lines[lineDays+1]), "Mon"); got != tt.weekdays {
			t.Errorf("width %d: weekday labels shown %v, want %v", tt.width, got, tt.weekdays)
		}
	}
}

func TestFooter(t *testing.T) {
	m := newModel(WithWeeks(year(today)), WithSize(80, 10), WithFocused(true))
	footer := ansi.Strip(m.lines[lineFooter])
	if !strings.HasPrefix(footer, statusText(m.grid[52][4].day)) || !strings.HasSuffix(footer, "More") {
		t.Fatalf("footer %q, want the status and the legend", footer)
	}
	// At 40 columns the status leaves no room for the legend.
	m.SetSize(40, 10)
	footer = ansi.Strip(m.lines[lineFooter])
	if strings.Contains(footer, "Less") || !strings.HasPrefix(footer, "No contributions on Thu, Sep 24") {
		t.Fatalf("footer %q, want only the status", footer)
	}
	m.Blur()
	if footer = ansi.Strip(m.lines[lineFooter]); strings.TrimSpace(footer) != "Less ■ ■ ■ ■ ■ More" {
		t.Fatalf("blurred footer %q, want only the legend", footer)
	}
}

func TestScrollFollowsCursor(t *testing.T) {
	m := newModel(WithWeeks(year(today)), WithSize(40, 10), WithFocused(true))
	if m.start != 35 {
		t.Fatalf("start %d, want the last 18 weeks", m.start)
	}
	for range 17 {
		m, _ = m.Update(press("h"))
	}
	if m.start != 35 {
		t.Fatalf("start %d, want no scroll while the cursor is in view", m.start)
	}
	m, _ = m.Update(press("h"))
	if m.start != 34 || m.cw != 34 {
		t.Fatalf("start %d and cursor week %d, want both 34", m.start, m.cw)
	}
	m.SetSize(40, 10)
	if m.start != 34 {
		t.Fatalf("start %d after a resize, want the cursor kept in view", m.start)
	}
	m, _ = m.Update(press("G"))
	if m.start != 35 {
		t.Fatalf("start %d, want the last weeks again", m.start)
	}
}

func TestText(t *testing.T) {
	tests := []struct{ got, want string }{
		{commas(0), "0"},
		{commas(999), "999"},
		{commas(1234), "1,234"},
		{commas(1234567), "1,234,567"},
		{commas(-12345), "-12,345"},
		{totalText(0, 0), "0 contributions in the last year"},
		{totalText(1, 0), "1 contribution in the last year"},
		{totalText(1234, 0), "1,234 contributions in the last year"},
		{totalText(214, 90), "214 contributions in the last 90 days"},
		{totalText(1, 1), "1 contribution in the last day"},
		{statusText(Day{Date: date(2026, 9, 23), Count: 12}), "12 contributions on Wed, Sep 23"},
		{statusText(Day{Date: date(2026, 9, 22), Count: 1}), "1 contribution on Tue, Sep 22"},
		{statusText(Day{Date: date(2026, 9, 22)}), "No contributions on Tue, Sep 22"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}
