package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:                  "0s",
		42 * time.Second:              "42s",
		65 * time.Second:              "1m 5s",
		time.Hour + 2*time.Minute + 9: "1h 2m",
	} {
		if got := Duration(d); got != want {
			t.Errorf("Duration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestSpan(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	if _, ok := Span(time.Time{}, now, now); ok {
		t.Error("what hasn't started has a span")
	}
	if d, _ := Span(now.Add(-time.Minute), time.Time{}, now); d != time.Minute {
		t.Errorf("what runs spans %v, want until now", d)
	}
}

func TestLines(t *testing.T) {
	if got := Spread("left side", "right", 12); ansi.StringWidth(got) != 12 || got != "left … right" {
		t.Errorf("Spread = %q", got)
	}
	if got := Fit("abcdef", 3); got != "abc" {
		t.Errorf("Fit = %q", got)
	}
	if got := FitLines([]string{"a"}, 2, 2); len(got) != 2 || got[1] != "  " {
		t.Errorf("FitLines = %q", got)
	}
	if got := Wrap("one two", 4); len(got) != 2 || got[0] != "one " {
		t.Errorf("Wrap = %q", got)
	}
	if got := FirstLine("a\nb"); got != "a" {
		t.Errorf("FirstLine = %q", got)
	}
}

func TestOneLine(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"a\nb\tc", "a b c"},
		{"a\r\n\x1b[31mb", "a b"},
		// Bidi controls would make the text read other than it is.
		{"CI \u202efdp.exe", "CI fdp.exe"},
		{"\u202aa\u202bb\u202cc\u202dd\u202ee", "abcde"},
		{"\u2066a\u2067b\u2068c\u2069", "abc"},
		{"a\u200eb\u200fc\u061cd", "abcd"},
		// Other invisible format characters go too.
		{"a\u200bb\u2060c\ufeffd\u00ade", "abcde"},
		// The joiners stay, since emoji and scripts need them.
		{"👩\u200d💻", "👩\u200d💻"},
		{"می\u200cخواهم", "می\u200cخواهم"},
		{"both\u202e\nlines", "both lines"},
	}
	for _, tt := range tests {
		if got := OneLine(tt.in); got != tt.want {
			t.Errorf("OneLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
