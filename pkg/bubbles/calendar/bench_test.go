package calendar

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func benchCalendar() Model {
	return newModel(WithWeeks(year(today)), WithSize(120, 10), WithFocused(true))
}

func BenchmarkView(b *testing.B) {
	m := benchCalendar()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	m := benchCalendar()
	// Box the keys once so the benchmark measures the calendar, not the
	// boxing.
	up, down := tea.Msg(press("up")), tea.Msg(press("down"))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Walk a week up and back down, across week boundaries.
		msg := up
		if i/7%2 == 1 {
			msg = down
		}
		i++
		m, _ = m.Update(msg)
	}
}

// BenchmarkSetWeeks measures laying out and rendering a year of days.
func BenchmarkSetWeeks(b *testing.B) {
	m := benchCalendar()
	weeks := year(today)
	b.ReportAllocs()
	for b.Loop() {
		m.SetWeeks(weeks)
	}
}
