package ui

import "testing"

func TestNumberOver(t *testing.T) {
	for num, want := range map[string]int{
		"#1": 0, "#1234": 0, "#12345": 0, "#123456": 1, "#1234567": 2,
	} {
		if got := NumberOver(num); got != want {
			t.Errorf("NumberOver(%q) = %d, want %d", num, got, want)
		}
	}
}
