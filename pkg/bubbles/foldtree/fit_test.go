package foldtree

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFit(t *testing.T) {
	tests := []struct {
		name          string
		lines         []string
		width, height int
		want          string
	}{
		{"short", []string{"ab"}, 4, 2, "ab  \n    "},
		{"long", []string{"abcdefgh", "x"}, 3, 2, "abc\nx  "},
		{"more lines than height", []string{"a", "b", "c"}, 1, 2, "a\nb"},
		{"height 0", []string{"a"}, 4, 0, ""},
		{"width 0", []string{"abc"}, 0, 2, "\n"},
		{"wide text", []string{"日本語"}, 5, 1, "日本 "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Fit(tt.lines, tt.width, tt.height)
			if got != tt.want {
				t.Errorf("Fit = %q, want %q", got, tt.want)
			}
			if tt.height > 0 {
				ls := strings.Split(got, "\n")
				if len(ls) != tt.height {
					t.Errorf("%d lines, want %d", len(ls), tt.height)
				}
				for _, l := range ls {
					if w := ansi.StringWidth(l); w != tt.width {
						t.Errorf("line %q is %d cells, want %d", l, w, tt.width)
					}
				}
			}
		})
	}
}
