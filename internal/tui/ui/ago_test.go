package ui

import (
	"testing"
	"time"
)

func TestAgo(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{-time.Hour, "now"},
		{30 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{49 * time.Hour, "2d"},
		{65 * 24 * time.Hour, "2mo"},
		{800 * 24 * time.Hour, "2y"},
	}
	for _, tt := range tests {
		if got := Ago(now.Add(-tt.ago), now); got != tt.want {
			t.Errorf("Ago(-%v) = %q, want %q", tt.ago, got, tt.want)
		}
	}
}

func TestAgoProse(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	if got := AgoProse(now.Add(-10*time.Second), now); got != "just now" {
		t.Errorf("AgoProse(-10s) = %q, want %q", got, "just now")
	}
	if got := AgoProse(now.Add(-3*time.Hour), now); got != "3h ago" {
		t.Errorf("AgoProse(-3h) = %q, want %q", got, "3h ago")
	}
}
