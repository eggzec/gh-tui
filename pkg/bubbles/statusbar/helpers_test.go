package statusbar

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestHint(t *testing.T) {
	t.Parallel()
	ks := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	ds := lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("2"))
	for _, tc := range []struct{ name, key, desc string }{
		{"both", "?", "help"},
		{"empty desc", "q", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Hint(tc.key, tc.desc, ks, ds, 3)
			want := ks.Render(tc.key) + " " + ds.Render(tc.desc)
			if len(got.Forms) != 1 || got.Forms[0] != want || got.Rank != 3 {
				t.Errorf("Hint = %q rank %d, want %q rank 3", got.Forms, got.Rank, want)
			}
		})
	}
}

func TestPair(t *testing.T) {
	t.Parallel()
	ls := lipgloss.NewStyle().Faint(true)
	vs := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))
	for _, tc := range []struct{ name, label, value string }{
		{"both", "core", "4 812/5 000"},
		{"empty value", "resets", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := ls.Render(tc.label+" ") + vs.Render(tc.value)
			if got := Pair(tc.label, tc.value, ls, vs); got != want {
				t.Errorf("Pair = %q, want %q", got, want)
			}
		})
	}
}
