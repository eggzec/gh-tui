package toast

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRoom(t *testing.T) {
	m := New(testDuration, testErrorDuration)
	for l := Info; l <= Warning; l++ {
		if got := m.Room(l); got != (Room{Share: 40, Lines: 3}) {
			t.Errorf("Room(%s) = %+v, want 40%% and 3 lines", l, got)
		}
	}
	if got := m.Room(Error); got != (Room{Share: 60, Lines: 5}) {
		t.Errorf("Room(error) = %+v, want 60%% and 5 lines", got)
	}

	m = New(testDuration, testErrorDuration, WithRoom(Info, Room{Share: 150, Lines: 0}), WithRoom(Level(9), Room{Share: 10, Lines: 1}))
	if got := m.Room(Info); got != (Room{Share: 100, Lines: 1}) {
		t.Errorf("Room(info) = %+v, want it kept to 100%% and 1 line", got)
	}
	m.SetRoom(Warning, Room{Share: -5, Lines: 7})
	if got := m.Room(Warning); got != (Room{Share: 1, Lines: 7}) {
		t.Errorf("Room(warning) = %+v, want it kept to 1%% and 7 lines", got)
	}
	m.SetRoom(Level(-1), Room{Share: 10, Lines: 1})
	if got := m.Room(Info); got != (Room{Share: 100, Lines: 1}) {
		t.Errorf("SetRoom of an unknown level changed info to %+v", got)
	}
}

// A room applies to its level only, and the lines it allows are the lines
// the toast shows.
func TestSetRoomWrapsToItsLines(t *testing.T) {
	long := strings.Repeat("word ", 60)
	m := New(testDuration, testErrorDuration, WithSize(80, 0))
	m.Push(Info, long)
	if n := strings.Count(m.View(), "\n") + 1; n != 3 {
		t.Errorf("info toast has %d lines, want 3", n)
	}
	m.SetRoom(Info, Room{Share: 50, Lines: 2})
	view := m.View()
	if n := strings.Count(view, "\n") + 1; n != 2 {
		t.Errorf("info toast has %d lines, want 2", n)
	}
	if w := ansi.StringWidth(strings.Split(view, "\n")[0]); w != 40 {
		t.Errorf("info toast is %d wide, want 40", w)
	}
}

func TestFits(t *testing.T) {
	m := New(testDuration, testErrorDuration, WithSize(80, 24))
	// At 80 columns an error has 48 cells, 39 of them for text once the
	// frame, the glyph and room for a count are taken: four words of these
	// a line, on five lines.
	tests := []struct {
		name  string
		level Level
		text  string
		want  bool
	}{
		{"short", Error, "Couldn't star: can't reach GitHub.", true},
		{"five lines", Error, strings.Repeat("abcdefghi ", 20), true},
		{"six lines", Error, strings.Repeat("abcdefghi ", 21), false},
		{"info has three lines", Info, strings.Repeat("abcdefghi ", 19), false},
		{"escapes don't count", Error, "\x1b[31m" + strings.Repeat("abcdefghi ", 20) + "\x1b[m", true},
	}
	for _, tt := range tests {
		if got := m.Fits(tt.level, tt.text); got != tt.want {
			t.Errorf("Fits(%s, %s) = %v, want %v", tt.level, tt.name, got, tt.want)
		}
	}
	if New(testDuration, testErrorDuration, WithSize(80, 2)).Fits(Error, strings.Repeat("abcdefghi ", 10)) {
		t.Error("a toast taller than the area fits")
	}
	if New(testDuration, testErrorDuration, WithSize(5, 0)).Fits(Error, "a") {
		t.Error("a toast fits where there is no room for text")
	}
}

// What Fits accepts shows whole: nothing of it is cut, even after it
// repeats, and a word longer than a line is broken rather than cut.
func TestFitsShowsWhole(t *testing.T) {
	texts := []string{
		"Couldn't load your teams: run gh auth refresh -s read:org, then restart gh-tui.",
		"Couldn't merge #5: something went wrong, see ~/.local/state/gh-tui/gh-tui.log.",
		"Couldn't open https://github.com/" + strings.Repeat("x", 90) + " at all.",
	}
	for width := 40; width <= 200; width += 7 {
		for _, text := range texts {
			m := New(testDuration, testErrorDuration, WithSize(width, 24))
			if !m.Fits(Error, text) {
				continue
			}
			for range 12 {
				m.Push(Error, text)
			}
			view := ansi.Strip(m.View())
			for line := range strings.SplitSeq(view, "\n") {
				if w := ansi.StringWidth(line); w > width {
					t.Errorf("%d: line is %d wide", width, w)
				}
			}
			// Only the text and its spaces are left once the edge, the
			// glyph and the count are gone.
			flat := strings.NewReplacer("▌", "", "✗", "", "×12", "", " ", "").Replace(view)
			if flat = strings.ReplaceAll(flat, "\n", ""); flat != strings.ReplaceAll(text, " ", "") {
				t.Errorf("%s: toast shows %q, want all of %q", strconv.Itoa(width), flat, text)
			}
		}
	}
}
