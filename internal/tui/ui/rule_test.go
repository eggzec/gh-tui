package ui

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
)

func TestRule(t *testing.T) {
	t.Parallel()
	for _, set := range []string{config.IconsASCII, config.IconsUnicode, config.IconsNerd} {
		ic := NewIcons(set)
		for _, tc := range []struct {
			name string
			n    int
			want string
		}{
			{"wide", 12, strings.Repeat(ic.Border.Top, 12)},
			{"one", 1, ic.Border.Top},
			{"zero", 0, ""},
			{"negative", -3, ""},
		} {
			t.Run(set+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				if got := Rule(ic, tc.n); got != tc.want {
					t.Errorf("Rule(%q, %d) = %q, want %q", set, tc.n, got, tc.want)
				}
			})
		}
	}
}
