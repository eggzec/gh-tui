package toast

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	long := "Could not close issue #7: resource not accessible by integration, " +
		"so the change was rolled back and the issue is open again"
	tests := []struct {
		name          string
		width, height int
		dark          bool
		pushes        []push
	}{
		{name: "info", width: 80, dark: true, pushes: []push{{Info, "Refreshing pull requests"}}},
		{name: "success", width: 80, dark: true, pushes: []push{{Success, "Merged #42"}}},
		{name: "warning", width: 80, dark: true, pushes: []push{{Warning, "Rate limit low: 12 left"}}},
		{name: "error", width: 80, dark: true, pushes: []push{{Error, "Could not star repo"}}},
		{name: "light", width: 80, pushes: []push{{Success, "Merged #42"}}},
		{
			name: "stack of three", width: 80, dark: true,
			pushes: []push{{Info, "Refreshing"}, {Success, "Merged #42"}, {Error, "Could not label #7"}},
		},
		{
			name: "dedup count", width: 80, dark: true,
			pushes: []push{{Error, "Network unreachable"}, {Error, "Network unreachable"}, {Error, "Network unreachable"}},
		},
		{name: "long text wraps and is cut", width: 80, dark: true, pushes: []push{{Error, long}}},
		{name: "minimum width", width: 40, dark: true, pushes: []push{{Info, long}}},
		{name: "narrower than the minimum", width: 16, dark: true, pushes: []push{{Info, long}}},
		{
			name: "short height keeps the newest", width: 80, height: 2, dark: true,
			pushes: []push{{Info, "a"}, {Info, "b"}, {Info, "c"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(WithStyles(DefaultStyles(tt.dark)), WithSize(tt.width, tt.height))
			for _, p := range tt.pushes {
				m.Push(p.level, p.text)
			}
			golden.RequireEqual(t, m.View())
		})
	}
}

func TestViewEmpty(t *testing.T) {
	if v := New(WithSize(80, 24)).View(); v != "" {
		t.Errorf("View() = %q, want empty", v)
	}
}

// The stack is one block, never wider than its share of the width or the
// width itself, with every line the same width.
func TestViewFitsTheWidth(t *testing.T) {
	for _, width := range []int{10, 16, 24, 40, 60, 80, 120, 200} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			m := New(WithSize(width, 0))
			m.Push(Info, "short")
			m.Push(Error, strings.Repeat("a very long message ", 10))
			m.Push(Success, "done")
			m.Push(Success, "done")
			limit := min(max(width*2/5, minWidth), width)
			lines := strings.Split(m.View(), "\n")
			w0 := ansi.StringWidth(lines[0])
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > limit || w != w0 {
					t.Errorf("line %d is %d wide, want %d and at most %d", i, w, w0, limit)
				}
			}
		})
	}
}

func overlayBackground(width, height int) string {
	lines := make([]string, height)
	for i := range lines {
		row := fmt.Sprintf("%02d \x1b[32m│\x1b[m row of the layout, with \x1b[1mstyled\x1b[m words ", i)
		lines[i] = ansi.Truncate(strings.Repeat(row, 3), width, "")
	}
	return strings.Join(lines, "\n")
}

func TestOverlay(t *testing.T) {
	m := New(WithSize(80, 24))
	m.Push(Info, "Refreshing")
	m.Push(Success, "Merged #42")
	m.Push(Error, "Could not label #7, rolled back")
	out := m.Overlay(overlayBackground(80, 24), 80, 24)
	golden.RequireEqual(t, out)

	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("overlay has %d lines, want 24", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 80 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
	// The toasts sit on the last rows, flush with the right edge.
	stack := strings.Split(m.View(), "\n")
	for i, s := range stack {
		row := ansi.Strip(lines[len(lines)-len(stack)+i])
		if !strings.HasSuffix(strings.TrimRight(row, " "), strings.TrimRight(ansi.Strip(s), " ")) {
			t.Errorf("row %q does not end with toast line %q", row, ansi.Strip(s))
		}
	}
}

func TestOverlayWithoutToasts(t *testing.T) {
	bg := overlayBackground(80, 5)
	if got := New(WithSize(80, 5)).Overlay(bg, 80, 5); got != bg {
		t.Error("Overlay changed the background without toasts")
	}
}

// A stack taller or wider than the area is clipped, keeping the newest.
func TestOverlayClipsToTheArea(t *testing.T) {
	m := New(WithSize(80, 0))
	m.Push(Info, "first")
	m.Push(Info, "second")
	m.Push(Info, "third")
	out := m.Overlay("short\nbackground\nwith\nextra\nlines", 10, 2)
	lines := strings.Split(ansi.Strip(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("overlay has %d lines, want 2: %q", len(lines), lines)
	}
	for i, want := range []string{"second", "third"} {
		if w := ansi.StringWidth(lines[i]); w > 10 {
			t.Errorf("line %d is %d wide", i, w)
		}
		if !strings.Contains(lines[i], want[:3]) {
			t.Errorf("line %d = %q, want part of %q", i, lines[i], want)
		}
	}
	// A short background is padded so that the stack still sits at the bottom.
	out = m.Overlay("top", 40, 5)
	lines = strings.Split(ansi.Strip(out), "\n")
	if len(lines) != 5 || lines[0] != "top" || lines[1] != "" || !strings.Contains(lines[4], "third") {
		t.Errorf("overlay = %q", lines)
	}
}
