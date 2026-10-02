package thread

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// requireFits fails unless out is exactly width by height cells.
func requireFits(t *testing.T, out string, width, height int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != height {
		t.Fatalf("got %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			t.Fatalf("line %d is %d cells wide, want %d: %q", i, w, width, l)
		}
	}
}

func TestViewLoadingDocument(t *testing.T) {
	m := newTest(newSource(1, 3), nil, 60, 6)
	out := m.View()
	requireFits(t, out, 60, 6)
	golden.RequireEqual(t, out)
}

func TestViewLoadingComments(t *testing.T) {
	m := newTest(newSource(1, 3), nil, 60, 24)
	// Keep the fetch in flight: don't run the command.
	_ = m.SetDocument(testHeader, testBody)
	out := m.View()
	requireFits(t, out, 60, 24)
	golden.RequireEqual(t, out)
}

func TestViewDocumentWithComments(t *testing.T) {
	m := loaded(t, newSource(1, 3), nil, 60, 30)
	out := m.View()
	requireFits(t, out, 60, 30)
	golden.RequireEqual(t, out)
}

func TestViewError(t *testing.T) {
	src := newSource(1, 3)
	src.fail[""] = 1
	m := loaded(t, src, nil, 60, 24)
	out := m.View()
	requireFits(t, out, 60, 24)
	golden.RequireEqual(t, out)
}

func TestViewEmpty(t *testing.T) {
	m := loaded(t, newSource(0, 0), nil, 60, 24)
	out := m.View()
	requireFits(t, out, 60, 24)
	golden.RequireEqual(t, out)
}

func TestViewEmptyText(t *testing.T) {
	m := loaded(t, newSource(0, 0), nil, 60, 24, WithEmptyText("No assets."))
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "No assets.") || strings.Contains(out, "No comments") {
		t.Errorf("empty thread says\n%s\nwant the empty text set", out)
	}
}

func TestViewTruncates80(t *testing.T) {
	src := newSource(1, 2)
	src.chunks[0][0].author = strings.Repeat("a-very-long-login-", 8)
	m := newTest(src, nil, 80, 24)
	header := "\x1b[1m" + strings.Repeat("A very long issue title that goes on ", 4) + "\x1b[m"
	m = drain(t, m, m.SetDocument(header, testBody+"\n\n"+strings.Repeat("x", 200)))
	out := m.View()
	requireFits(t, out, 80, 24)
	golden.RequireEqual(t, out)
}

func TestViewFits(t *testing.T) {
	sizes := []struct{ w, h int }{{1, 1}, {10, 3}, {40, 10}, {80, 24}, {120, 50}}
	for _, s := range sizes {
		m := loaded(t, newSource(3, 4), nil, s.w, s.h)
		for range 3 {
			requireFits(t, m.View(), s.w, s.h)
			m = press(t, m, "d")
		}
	}
}

func TestViewZeroSize(t *testing.T) {
	m := loaded(t, newSource(1, 1), nil, 0, 0)
	if out := m.View(); out != "" {
		t.Fatalf("View() = %q, want empty", out)
	}
}

// The texts while loading and the cut of a long line end in the ellipsis
// of the styles.
func TestViewEllipsis(t *testing.T) {
	st := DefaultStyles(true)
	st.Ellipsis, st.Pointer = "...", ">"
	m := newTest(newSource(1, 3), nil, 40, 6)
	m.SetStyles(st)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Loading...") {
		t.Errorf("view while loading lacks the ellipsis:\n%s", v)
	}
	header := strings.Repeat("A very long issue title that goes on ", 4)
	m = drain(t, m, m.SetDocument(header, testBody))
	if first, _, _ := strings.Cut(ansi.Strip(m.View()), "\n"); !strings.HasSuffix(first, "...") {
		t.Errorf("cut header %q doesn't end in the ellipsis", first)
	}
}
