package ui

import (
	"testing"
	"time"
)

func TestAgo(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"now", now, "now"},
		{"clock skew, a second ahead", now.Add(time.Second), "now"},
		{"clock skew, an hour ahead", now.Add(time.Hour), "now"},
		{"under a minute", now.Add(-59 * time.Second), "now"},
		{"a minute", now.Add(-time.Minute), "1m"},
		{"minutes", now.Add(-5 * time.Minute), "5m"},
		{"under an hour", now.Add(-time.Hour + time.Second), "59m"},
		{"an hour", now.Add(-time.Hour), "1h"},
		{"under a day", now.Add(-24*time.Hour + time.Second), "23h"},
		{"a day", now.Add(-24 * time.Hour), "1d"},
		{"days", now.Add(-49 * time.Hour), "2d"},
		{"under a month", now.Add(-30*24*time.Hour + time.Second), "29d"},
		{"a month", now.Add(-30 * 24 * time.Hour), "1mo"},
		{"months", now.Add(-65 * 24 * time.Hour), "2mo"},
		{"under a year", now.Add(-365*24*time.Hour + time.Second), "12mo"},
		{"a year ago", now.AddDate(-1, 0, 0), "1y"},
		{"years", now.Add(-800 * 24 * time.Hour), "2y"},
		{"another zone", now.Add(-3 * time.Hour).In(time.FixedZone("UTC+5", 5*60*60)), "3h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Ago(tt.at, now); got != tt.want {
				t.Errorf("Ago(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

func TestAgoProse(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"now", now, "just now"},
		{"clock skew", now.Add(time.Minute), "just now"},
		{"seconds", now.Add(-10 * time.Second), "just now"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"a year ago", now.AddDate(-1, 0, 0), "1y ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AgoProse(tt.at, now); got != tt.want {
				t.Errorf("AgoProse(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

func TestClock(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	east := time.FixedZone("UTC+5", 5*60*60)
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"now", now, "12:00"},
		{"later today", now.Add(3 * time.Hour), "15:00"},
		{"earlier today", now.Add(-12 * time.Hour), "00:00"},
		{"tomorrow, under a day away", now.Add(13 * time.Hour), "Sep 24 01:00"},
		{"yesterday", now.Add(-13 * time.Hour), "Sep 22 23:00"},
		{"another day in its zone", now.Add(8 * time.Hour).In(east), "Sep 24 01:00"},
		{"next year", now.AddDate(1, 0, 0), "Sep 23 2027 12:00"},
		{"a year ago", now.AddDate(-1, 0, 0), "Sep 23 2025 12:00"},
		{"the next midnight", time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), "Sep 24 00:00"},
		{"the last midnight", time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC), "00:00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clock(tt.at, now); got != tt.want {
				t.Errorf("Clock(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}

// Around midnight and the new year, the day and the year are those of the
// time told, even when it is minutes away.
func TestClockAtTheTurn(t *testing.T) {
	eve := time.Date(2026, 12, 31, 23, 50, 0, 0, time.UTC)
	newYear := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		at, now time.Time
		want    string
	}{
		{"new year's day, from the eve", newYear.Add(15 * time.Minute), eve, "Jan 1 2027 00:15"},
		{"the eve, from new year's day", eve, newYear.Add(10 * time.Minute), "Dec 31 2026 23:50"},
		{"midnight itself, from the eve", newYear, eve, "Jan 1 2027 00:00"},
		{"the eve, at midnight", eve, newYear, "Dec 31 2026 23:50"},
		{"midnight, at midnight", newYear, newYear, "00:00"},
		{"a minute before midnight, at midnight", time.Date(2026, 9, 23, 23, 59, 0, 0, time.UTC), time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), "Sep 23 23:59"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clock(tt.at, tt.now); got != tt.want {
				t.Errorf("Clock(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}
