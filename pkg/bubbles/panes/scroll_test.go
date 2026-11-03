package panes

import (
	"strings"
	"testing"
)

func TestScrolls(t *testing.T) {
	t.Parallel()
	var s Scrolls[string]
	if s.Get("a") != 0 || s.Has("a") {
		t.Fatal("zero value should hold nothing")
	}
	s.Set("a", 7)
	if !s.Has("a") || s.Get("a") != 7 {
		t.Fatalf("Get = %d, want 7", s.Get("a"))
	}
	tests := []struct {
		name     string
		off      int
		lines, h int
		want     int
	}{
		{"inside", 3, 10, 5, 3},
		{"past the end", 9, 10, 5, 5},
		{"empty body", 4, 0, 5, 0},
		{"zero height", 4, 10, 0, 4},
		{"zero height past end", 40, 10, 0, 10},
		{"negative", -3, 10, 5, 0},
		{"short body", 2, 3, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var s Scrolls[int]
			s.Set(1, tt.off)
			if got := s.Clamp(1, tt.lines, tt.h); got != tt.want {
				t.Errorf("Clamp = %d, want %d", got, tt.want)
			}
			if s.Get(1) != tt.off {
				t.Error("Clamp changed the kept offset")
			}
		})
	}
	s.Reset("a")
	if s.Has("a") {
		t.Error("Reset kept the offset")
	}
	s.Reset("never set")
}

func TestWindow(t *testing.T) {
	t.Parallel()
	body := "a\nb\nc\nd\ne"
	tests := []struct {
		name    string
		body    string
		off, h  int
		want    string
		wantOff int
	}{
		{"top", body, 0, 2, "a\nb\nc\nd\ne", 0},
		{"middle", body, 2, 2, "c\nd\ne", 2},
		{"past the end", body, 9, 2, "d\ne", 3},
		{"empty body", "", 3, 4, "", 0},
		{"zero height", body, 9, 0, "", 5},
		{"negative", body, -1, 2, body, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, off := Window(tt.body, tt.off, tt.h)
			if got != tt.want || off != tt.wantOff {
				t.Errorf("Window = %q, %d; want %q, %d", got, off, tt.want, tt.wantOff)
			}
		})
	}
}

func BenchmarkWindow(b *testing.B) {
	body := strings.Repeat("a line of the preview\n", 500)
	b.ReportAllocs()
	for b.Loop() {
		Window(body, 200, 30)
	}
}
