package termimg

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The diacritics are the protocol's, in its order.
func TestDiacriticTable(t *testing.T) {
	f, err := os.Open("testdata/rowcolumn-diacritics.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var want []rune
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if l := sc.Text(); l != "" && !strings.HasPrefix(l, "#") {
			r, err := strconv.ParseUint(l, 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			want = append(want, rune(r))
		}
	}
	if len(want) != MaxCells || len(diacritics) != MaxCells {
		t.Fatalf("%d diacritics in the spec, %d here, MaxCells %d", len(want), len(diacritics), MaxCells)
	}
	for i, r := range want {
		if diacritics[i] != r {
			t.Errorf("diacritic %d is %U, want %U", i, diacritics[i], r)
		}
		if got := diacriticIndex(r); got != i {
			t.Errorf("diacriticIndex(%U) = %d, want %d", r, got, i)
		}
	}
}

func TestRowsGolden(t *testing.T) {
	got := Rows(NewID(2, 42), 2, 2)
	want := []string{
		"\x1b[38;5;42m\U0010EEEE\u0305\u0305\u030e\U0010EEEE\u0305\u030d\u030e\x1b[39m",
		"\x1b[38;5;42m\U0010EEEE\u030d\u0305\u030e\U0010EEEE\u030d\u030d\u030e\x1b[39m",
	}
	if len(got) != len(want) {
		t.Fatalf("%d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
	if n := len(Rows(NewID(1, 1), 0, MaxCells+1)); n != MaxCells {
		t.Errorf("clamped to %d rows, want %d", n, MaxCells)
	}
}

// Every cell names its own row, column and ID, so reading the cells back
// gives the grid.
func TestRowsRoundTrip(t *testing.T) {
	id := NewID(255, 7)
	cols, rows := MaxCells, 3
	for r, line := range Rows(id, cols, rows) {
		s := strings.TrimSuffix(strings.TrimPrefix(line, "\x1b[38;5;7m"), fgReset)
		for c := 0; s != ""; c++ {
			row, col, msb, n, ok := Cell(s)
			if !ok || row != r || col != c || msb != id.MSB() {
				t.Fatalf("cell %d,%d reads %d,%d msb %d ok %v", r, c, row, col, msb, ok)
			}
			s = s[n:]
		}
	}
	full := "\U0010EEEE\u0305\u0305\u030e"
	for _, s := range []string{
		"", "x", "\U0010EEEE", "\U0010EEEE\u0305\u0305", "\U0010EEEE\u0305\u0305a",
		// Cut inside the last diacritic, and inside the placeholder.
		full[:len(full)-1], full[:3],
		// Invalid UTF-8 in place of a diacritic.
		"\U0010EEEE\u0305\xcc\u030e", "\U0010EEEE\xff\xff\xff\xff",
	} {
		if _, _, _, _, ok := Cell(s); ok {
			t.Errorf("Cell(%q) is ok", s)
		}
	}
}

// A row is as wide as its cells by every measure the views use, and a cut
// keeps whole cells, each with its diacritics and color.
func TestRowsWidthAndCuts(t *testing.T) {
	id := NewID(3, 99)
	for _, cols := range []int{1, 5, 80} {
		row := Rows(id, cols, 1)[0]
		for name, w := range map[string]int{
			"ansi.StringWidth":   ansi.StringWidth(row),
			"ansi.StringWidthWc": ansi.StringWidthWc(row),
			"lipgloss.Width":     lipgloss.Width(row),
		} {
			if w != cols {
				t.Errorf("%s of %d cells = %d", name, cols, w)
			}
		}
	}
	row := Rows(id, 5, 1)[0]
	wantCells := func(t *testing.T, s string, from, n int) {
		t.Helper()
		if w := ansi.StringWidth(s); w != n {
			t.Errorf("%d cells wide, want %d: %q", w, n, s)
		}
		if !strings.HasPrefix(s, "\x1b[38;5;99m") {
			t.Errorf("lost its color: %q", s)
		}
		plain := ansi.Strip(s)
		for c := from; c < from+n; c++ {
			_, col, msb, size, ok := Cell(plain)
			if !ok || col != c || msb != 3 {
				t.Fatalf("cell %d reads col %d msb %d ok %v in %q", c, col, msb, ok, s)
			}
			plain = plain[size:]
		}
		if plain != "" {
			t.Errorf("left over: %q", plain)
		}
	}
	t.Run("truncate", func(t *testing.T) { wantCells(t, ansi.Truncate(row, 3, ""), 0, 3) })
	t.Run("truncate left", func(t *testing.T) { wantCells(t, ansi.TruncateLeft(row, 2, ""), 2, 3) })
	t.Run("cut", func(t *testing.T) { wantCells(t, ansi.Cut(row, 1, 4), 1, 3) })
}

func BenchmarkRows(b *testing.B) {
	id := NewID(1, 1)
	b.ReportAllocs()
	for b.Loop() {
		Rows(id, 80, 20)
	}
}
