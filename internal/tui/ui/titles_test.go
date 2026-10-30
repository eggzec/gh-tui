package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTitle(t *testing.T) {
	t.Parallel()
	sp := func(n int) string { return strings.Repeat(" ", n) }
	base := TitleSpec{Name: "Branches", Sep: " | ", Ellipsis: "~"}
	with := func(f func(*TitleSpec)) TitleSpec {
		s := base
		f(&s)
		return s
	}
	tests := []struct {
		name string
		spec TitleSpec
		want string
	}{
		{"bare", base, "Branches"},
		{"bare fills width", with(func(s *TitleSpec) { s.Width = 12 }), "Branches" + sp(4)},
		{"key", with(func(s *TitleSpec) { s.Key = "2" }), "[2] Branches"},
		{"chips", with(func(s *TitleSpec) { s.Chips = []string{"a", "b"} }), "Branches | a | b"},
		{"count without width", with(func(s *TitleSpec) { s.Count = "12" }), "Branches | 12"},
		{"count right aligned", with(func(s *TitleSpec) { s.Count, s.Width = "12", 20 }), "Branches" + sp(10) + "12"},
		{"all", with(func(s *TitleSpec) {
			s.Key, s.Chips, s.Count, s.Width = "1", []string{"a"}, "3", 24
		}), "[1] Branches | a" + sp(7) + "3"},
		{"cuts the last chips first", with(func(s *TitleSpec) {
			s.Key, s.Chips, s.Count, s.Width = "1", []string{"abc", "def"}, "3", 20
		}), "[1] Branches | abc 3"},
		{"cuts all chips", with(func(s *TitleSpec) {
			s.Key, s.Chips, s.Count, s.Width = "1", []string{"abc", "def"}, "3", 16
		}), "[1] Branches" + sp(3) + "3"},
		{"cuts the key before the name", with(func(s *TitleSpec) {
			s.Key, s.Count, s.Width = "1", "3", 12
		}), "Branches" + sp(3) + "3"},
		{"cuts the name last", with(func(s *TitleSpec) { s.Count, s.Width = "3", 6 }), "Bra~ 3"},
	}
	for _, tc := range tests {
		got := Title(tc.spec)
		if got != tc.want {
			t.Errorf("%s: Title = %q, want %q", tc.name, got, tc.want)
		}
		if tc.spec.Width > 0 && ansi.StringWidth(got) != tc.spec.Width {
			t.Errorf("%s: width %d, want %d", tc.name, ansi.StringWidth(got), tc.spec.Width)
		}
	}
}
