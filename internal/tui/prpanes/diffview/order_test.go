package diffview

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestRangeLabel(t *testing.T) {
	t.Parallel()
	const a, b = "abc1234567890", "def5678901234"
	for _, tc := range []struct {
		name            string
		total, from, to int
		first, last     string
		want            string
	}{
		{"every commit", 5, 0, 4, a, b, "all 5 commits · abc1234…def5678"},
		{"two commits", 2, 0, 1, a, b, "all 2 commits · abc1234…def5678"},
		{"a sub-range", 5, 1, 3, a, b, "3 of 5 commits · abc1234…def5678"},
		{"a sub-range at the start", 5, 0, 2, a, b, "3 of 5 commits · abc1234…def5678"},
		{"one commit of several", 5, 2, 2, a, a, "abc1234"},
		{"total of one", 1, 0, 0, a, a, "abc1234"},
		{"short ids stay", 3, 0, 2, "abc", "def", "all 3 commits · abc…def"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Unicode.RangeLabel(tc.total, tc.from, tc.to, tc.first, tc.last); got != tc.want {
				t.Errorf("RangeLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOrder(t *testing.T) {
	t.Parallel()
	files := func(paths ...string) []core.CommitFile {
		fs := make([]core.CommitFile, len(paths))
		for i, p := range paths {
			fs[i] = core.CommitFile{Path: p}
		}
		return fs
	}
	for _, tc := range []struct {
		name string
		in   []core.CommitFile
		want []string
	}{
		{"none", nil, []string{}},
		{"unsorted", files("c.go", "a.go", "b.go"), []string{"a.go", "b.go", "c.go"}},
		{"duplicates", files("b.go", "a.go", "b.go", "a.go"), []string{"a.go", "b.go"}},
		// Byte order puts "-" and "." before "/", and capitals before lowercase.
		{
			"nested paths in byte order",
			files("a/b.go", "a.go", "a-b.go", "a/a/z.go", "B.go", "b.go"),
			[]string{"B.go", "a-b.go", "a.go", "a/a/z.go", "a/b.go", "b.go"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := slices.Clone(tc.in)
			if got := Order(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("Order = %q, want %q", got, tc.want)
			}
			if !slices.Equal(in, tc.in) {
				t.Errorf("Order changed its input: %v, was %v", tc.in, in)
			}
		})
	}
}

func TestStep(t *testing.T) {
	t.Parallel()
	order := []string{"a", "b", "c"}
	for _, tc := range []struct {
		name   string
		path   string
		d      int
		want   string
		wantOK bool
	}{
		{"middle forward", "b", 1, "c", true},
		{"middle back", "b", -1, "a", true},
		{"first back", "a", -1, "", false},
		{"last forward", "c", 1, "", false},
		{"missing path", "x", 1, "", false},
		{"two ahead", "a", 2, "c", true},
		{"past the end by two", "b", 2, "", false},
		{"stay", "b", 0, "b", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := Step(order, tc.path, tc.d)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("Step(%q, %d) = %q, %v; want %q, %v", tc.path, tc.d, got, ok, tc.want, tc.wantOK)
			}
		})
	}
	if got, ok := Step(nil, "a", 1); ok || got != "" {
		t.Errorf("Step on no order = %q, %v", got, ok)
	}
}

func TestPlace(t *testing.T) {
	t.Parallel()
	order := []string{"a", "b", "c"}
	for _, tc := range []struct {
		path         string
		wantI, wantN int
	}{
		{"a", 1, 3},
		{"c", 3, 3},
		{"x", 0, 3},
	} {
		if i, n := Place(order, tc.path); i != tc.wantI || n != tc.wantN {
			t.Errorf("Place(%q) = %d, %d; want %d, %d", tc.path, i, n, tc.wantI, tc.wantN)
		}
	}
	if i, n := Place(nil, "a"); i != 0 || n != 0 {
		t.Errorf("Place on no order = %d, %d", i, n)
	}
}
