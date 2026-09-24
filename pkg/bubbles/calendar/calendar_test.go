package calendar

import (
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// today is the last day of the sample year, a Thursday.
var today = time.Date(2026, time.September, 24, 0, 0, 0, 0, time.UTC)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// year returns the days from a year before end to end, in weeks from
// Sunday, with counts that vary from day to day.
func year(end time.Time) [][]Day {
	var weeks [][]Day
	for d, i := end.AddDate(-1, 0, 0), 0; !d.After(end); d, i = d.AddDate(0, 0, 1), i+1 {
		if len(weeks) == 0 || d.Weekday() == time.Sunday {
			weeks = append(weeks, nil)
		}
		n := i * 7919 % 13
		if i%5 == 0 {
			n = 0
		}
		level := 0
		if n > 0 {
			level = 1 + min(n/4, 3)
		}
		weeks[len(weeks)-1] = append(weeks[len(weeks)-1], Day{Date: d, Count: n, Level: level})
	}
	return weeks
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// keys presses ks in turn.
func keys(tb testing.TB, m Model, ks ...string) Model {
	tb.Helper()
	for _, k := range ks {
		m, _ = m.Update(press(k))
	}
	return m
}

func selected(tb testing.TB, m Model) time.Time {
	tb.Helper()
	d, ok := m.Selected()
	if !ok {
		tb.Fatal("no day selected")
	}
	return d.Date
}

func TestSetWeeks(t *testing.T) {
	m := New(WithWeeks(year(today)), WithSize(120, 10))
	weeks := m.Weeks()
	if len(weeks) != 53 {
		t.Fatalf("%d weeks, want 53", len(weeks))
	}
	// A year back from a Thursday starts on a Wednesday.
	if len(weeks[0]) != 4 || weeks[0][0].Date.Weekday() != time.Wednesday {
		t.Fatalf("first week %v, want Wednesday to Saturday", weeks[0])
	}
	if len(weeks[52]) != 5 {
		t.Fatalf("last week has %d days, want Sunday to Thursday", len(weeks[52]))
	}
	if got := selected(t, m); !got.Equal(today) {
		t.Fatalf("cursor on %v, want the last day", got)
	}
	sum := 0
	for _, w := range weeks {
		for _, d := range w {
			sum += d.Count
		}
	}
	if m.Total() != sum {
		t.Fatalf("total %d, want the sum %d", m.Total(), sum)
	}
	m.SetTotal(1234)
	if m.Total() != 1234 {
		t.Fatalf("total %d, want 1234", m.Total())
	}
	m.SetTotal(-5)
	if m.Total() != sum {
		t.Fatalf("total %d after a negative total, want the sum %d", m.Total(), sum)
	}
}

func TestSetWeeksNormalizes(t *testing.T) {
	in := [][]Day{
		{},
		{{Date: date(2026, 9, 20), Level: 9}, {Date: date(2026, 9, 21), Level: -1}},
	}
	m := New(WithWeeks(in))
	weeks := m.Weeks()
	if len(weeks) != 1 {
		t.Fatalf("%d weeks, want the empty one skipped", len(weeks))
	}
	if weeks[0][0].Level != Levels-1 || weeks[0][1].Level != 0 {
		t.Fatalf("levels %d and %d, want them clamped", weeks[0][0].Level, weeks[0][1].Level)
	}
	// The model keeps its own days.
	in[1][0].Count = 99
	if m.Weeks()[0][0].Count != 0 {
		t.Fatal("changing the caller's days changed the calendar")
	}
}

func TestSetWeeksKeepsCursor(t *testing.T) {
	m := New(WithWeeks(year(today)), WithFocused(true))
	m = keys(t, m, "h", "h")
	want := date(2026, 9, 10)
	if got := selected(t, m); !got.Equal(want) {
		t.Fatalf("cursor on %v, want %v", got, want)
	}
	m.SetWeeks(year(today.AddDate(0, 0, 1)))
	if got := selected(t, m); !got.Equal(want) {
		t.Fatalf("cursor on %v after new data, want it to stay on %v", got, want)
	}
	m.SetWeeks(year(today.AddDate(1, 0, 0)))
	if got := selected(t, m); !got.Equal(today.AddDate(1, 0, 0)) {
		t.Fatalf("cursor on %v, want the last day once its date is gone", got)
	}
}

func TestCursor(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want time.Time
		// sent is whether the last key sent a SelectMsg.
		sent bool
	}{
		{"starts on the last day", nil, today, false},
		{"up goes to the day before", []string{"k"}, date(2026, 9, 23), true},
		{"arrows work too", []string{"up", "left"}, date(2026, 9, 16), true},
		{"down stops at the last day", []string{"j"}, today, false},
		{"right stops at the last week", []string{"l"}, today, false},
		{"left goes to the week before", []string{"h"}, date(2026, 9, 17), true},
		{"up from Sunday goes to Saturday", []string{"k", "k", "k", "k", "k"}, date(2026, 9, 19), true},
		{"down from Saturday goes to Sunday", []string{"h", "j", "j", "j"}, date(2026, 9, 20), true},
		{"first day", []string{"g"}, date(2025, 9, 24), true},
		{"up stops at the first day", []string{"home", "k"}, date(2025, 9, 24), false},
		{"left stops at the first week", []string{"g", "h"}, date(2025, 9, 24), false},
		{"left into a partial week goes to its nearest day", []string{"g", "l", "k", "k", "k", "h"}, date(2025, 9, 24), true},
		{"right into a partial week goes to its nearest day", []string{"h", "j", "j", "l"}, today, true},
		{"last day", []string{"g", "G"}, today, true},
		{"end", []string{"g", "end"}, today, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(WithWeeks(year(today)), WithSize(80, 10), WithFocused(true))
			var cmd tea.Cmd
			if n := len(tt.keys); n > 0 {
				m = keys(t, m, tt.keys[:n-1]...)
				m, cmd = m.Update(press(tt.keys[n-1]))
			}
			if got := selected(t, m); !got.Equal(tt.want) {
				t.Fatalf("cursor on %s, want %s", got.Format(time.DateOnly), tt.want.Format(time.DateOnly))
			}
			if (cmd != nil) != tt.sent {
				t.Fatalf("sent a message: %v, want %v", cmd != nil, tt.sent)
			}
			if cmd == nil {
				return
			}
			sel, ok := cmd().(SelectMsg)
			if !ok || sel.ID != m.ID() || !sel.Day.Date.Equal(tt.want) {
				t.Fatalf("message %#v, want a SelectMsg from %d for the new day", sel, m.ID())
			}
		})
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := New(WithWeeks(year(today)), WithSize(80, 10))
	if m.Focused() {
		t.Fatal("a new calendar should start blurred")
	}
	m, cmd := m.Update(press("h"))
	if cmd != nil || !selected(t, m).Equal(today) {
		t.Fatal("a blurred calendar moved its cursor")
	}
	m.Focus()
	m, cmd = m.Update(press("h"))
	if cmd == nil || !selected(t, m).Equal(date(2026, 9, 17)) {
		t.Fatal("a focused calendar did not move its cursor")
	}
}

func TestIDsDiffer(t *testing.T) {
	a, b := New(), New()
	if a.ID() == b.ID() {
		t.Fatal("two calendars share an ID")
	}
}

func TestSelect(t *testing.T) {
	m := New(WithWeeks(year(today)), WithSize(40, 10))
	if !m.Select(date(2025, 10, 1)) {
		t.Fatal("Select found no day")
	}
	if m.start != 1 {
		t.Fatalf("start %d, want the weeks to scroll to the cursor", m.start)
	}
	if m.Select(date(2020, 1, 1)) {
		t.Fatal("Select found a day outside the data")
	}
}

func TestCopiesAreIndependent(t *testing.T) {
	m := New(WithWeeks(year(today)), WithSize(80, 10), WithFocused(true))
	before := m.View()
	moved, _ := m.Update(press("k"))
	if m.View() != before {
		t.Fatal("moving a copy changed the original")
	}
	if moved.View() == before {
		t.Fatal("moving the cursor did not change the view")
	}
}
