package ui

import (
	"slices"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
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
	if got := OneLine("a\nb\tc"); got != "a b c" {
		t.Errorf("OneLine = %q", got)
	}
	if got := FirstLine("a\nb"); got != "a" {
		t.Errorf("FirstLine = %q", got)
	}
}

func TestFreeKeys(t *testing.T) {
	b := key.NewBinding(key.WithKeys("down", "j", "f"), key.WithHelp("↓/j", "down"))
	taken := key.NewBinding(key.WithKeys("f"))
	got := FreeKeys(b, taken, key.NewBinding(key.WithKeys("j"), key.WithDisabled()))
	if !slices.Equal(got.Keys(), []string{"down", "j"}) || got.Help().Key != "↓/j" || got.Help().Desc != "down" {
		t.Errorf("FreeKeys = %v %+v", got.Keys(), got.Help())
	}
	if FreeKeys(taken, taken).Enabled() {
		t.Error("a binding without keys is enabled")
	}
}
