package diff

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// ruler is a code line that is wider than any test view.
var ruler = "ABC" + strings.Repeat("0123456789", 12)

// wideBlock is a block under line 2 of a.go: a line of prose that fits, and a
// code line wider than the width it is given. It records that width.
func wideBlock(seen *int) Block {
	return Block{
		ID:     "wide",
		Anchor: Pos{Path: "a.go", Side: NewSide, Line: 2},
		Render: func(width int, _ bool) []string {
			if seen != nil {
				*seen = width
			}
			return []string{"prose here", ruler}
		},
	}
}

func TestWideBlockScrolls(t *testing.T) {
	t.Parallel()
	m := withBlocks(t, blockFiles(), 60, 16, wideBlock(nil))
	if want := ansi.StringWidth(ruler) - m.blockWidth(); m.maxLeft() != want {
		t.Fatalf("maxLeft = %d, want %d: it does not count the wide block row", m.maxLeft(), want)
	}
	before := ansi.Strip(m.View())
	if !strings.Contains(before, "prose here") || !strings.Contains(before, "ABC0123") {
		t.Fatalf("view lacks the block rows:\n%s", before)
	}
	m = keys(t, m, "l")
	if m.left == 0 {
		t.Fatal("l did not scroll")
	}
	after := ansi.Strip(m.View())
	if strings.Contains(after, "ABC0123") {
		t.Errorf("the wide row did not scroll:\n%s", after)
	}
	if !strings.Contains(after, "prose here") {
		t.Errorf("the prose row did not stay fixed:\n%s", after)
	}
	assertFits(t, m.View(), 60, 16)
	// At the far end the row ends at the right edge.
	m.left = m.maxLeft()
	if v := ansi.Strip(m.View()); !strings.Contains(v, "89 ") && !strings.Contains(v, "789\n") {
		t.Errorf("the end of the wide row is not shown:\n%s", v)
	}
}

func TestWideBlockWraps(t *testing.T) {
	t.Parallel()
	m := withBlocks(t, blockFiles(), 60, 24, wideBlock(nil))
	m.SetWrap(true)
	w := m.blockWidth()
	if ansi.StringWidth(ruler) < 2*w {
		t.Fatalf("setup: ruler of %d cells is not 2 rows at width %d", ansi.StringWidth(ruler), w)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "prose here") {
		t.Errorf("the prose row is missing:\n%s", v)
	}
	// Every part of the code line shows, on rows of their own.
	var got strings.Builder
	for l := range strings.SplitSeq(v, "\n") {
		if i := strings.Index(l, "ABC"); i >= 0 {
			l = l[i:]
		} else if !strings.Contains(l, "0123456789") {
			continue
		}
		got.WriteString(strings.TrimSpace(l))
	}
	if !strings.Contains(got.String(), ruler) {
		t.Errorf("the wrapped rows do not hold the whole code line %q:\n%s", ruler, v)
	}
	assertFits(t, m.View(), 60, 24)
	m = keys(t, m, "l")
	if m.left != 0 {
		t.Errorf("l scrolled a wrapped view to %d", m.left)
	}
}

func TestLineNumbersBlockWidth(t *testing.T) {
	t.Parallel()
	var seen int
	m := withBlocks(t, blockFiles(), 80, 16, wideBlock(&seen))
	on := seen
	if on != m.blockWidth() {
		t.Fatalf("block rendered at %d, want %d", on, m.blockWidth())
	}
	m.left = m.maxLeft()
	scrolled := m.left
	m.SetLineNumbers(false)
	if seen <= on {
		t.Errorf("block width with numbers off = %d, not wider than %d", seen, on)
	}
	if seen != m.blockWidth() {
		t.Errorf("block rendered at %d, want %d", seen, m.blockWidth())
	}
	if m.left != m.maxLeft() || m.left >= scrolled {
		t.Errorf("left = %d (max %d), want it clamped below %d", m.left, m.maxLeft(), scrolled)
	}
	m.SetLineNumbers(true)
	if seen != on {
		t.Errorf("block width with numbers back on = %d, want %d", seen, on)
	}
}

func TestOptionKeys(t *testing.T) {
	t.Parallel()
	load1 := func(t *testing.T) Model {
		t.Helper()
		return load(t, &source{size: 5, files: []File{goFile("a.go", 1)}}, WithSize(80, 8))
	}
	t.Run("toggle", func(t *testing.T) {
		t.Parallel()
		m := keys(t, load1(t), "-")
		if !m.ChoosingOption() {
			t.Fatal("- does not wait for an option")
		}
		m = keys(t, m, "N")
		if m.ChoosingOption() || m.LineNumbers() || m.flash != noteNoNumbers {
			t.Errorf("after -N: choosing %v, numbers %v, flash %q", m.ChoosingOption(), m.LineNumbers(), m.flash)
		}
		m = keys(t, m, "-", "N")
		if !m.LineNumbers() || m.flash != noteNumbers {
			t.Errorf("after -N again: numbers %v, flash %q", m.LineNumbers(), m.flash)
		}
	})
	t.Run("no such option", func(t *testing.T) {
		t.Parallel()
		m := keys(t, load1(t), "-", "z")
		if m.ChoosingOption() || !m.LineNumbers() || m.flash != noteNoOption+"z" {
			t.Errorf("after -z: choosing %v, numbers %v, flash %q", m.ChoosingOption(), m.LineNumbers(), m.flash)
		}
		m = keys(t, m, "j")
		if m.flash != "" {
			t.Errorf("flash %q outlives the next key", m.flash)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		t.Parallel()
		m := keys(t, load1(t), "-", "esc")
		if m.ChoosingOption() || !m.LineNumbers() || m.flash != "" {
			t.Errorf("after -esc: choosing %v, numbers %v, flash %q", m.ChoosingOption(), m.LineNumbers(), m.flash)
		}
	})
}
