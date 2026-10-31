package ownerui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestDigits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		keys      []string
		cur       int
		want      string
		wantWidth int
	}{
		{"all bound", []string{"1", "2", "3"}, 1, "1 [2] 3", 7},
		{"unbound skipped", []string{"1", "", "3"}, 2, "1 [3]", 5},
		{"unbound first", []string{"", "2", "3"}, 2, "2 [3]", 5},
		{"focused unbound", []string{"1", "", "3"}, 1, "1 3", 3},
		{"all unbound", []string{"", "", ""}, 0, "", 0},
		{"no keys", nil, 0, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, w := Digits(tt.keys, tt.cur, ui.Paint{}, ui.Paint{})
			if got != tt.want || w != tt.wantWidth {
				t.Errorf("Digits = %q, %d; want %q, %d", got, w, tt.want, tt.wantWidth)
			}
			if strings.Contains(got, "  ") {
				t.Errorf("Digits %q has a double space", got)
			}
		})
	}
}

func TestFoot(t *testing.T) {
	t.Parallel()
	keys := []string{"1", "2", "3"}
	tiers := [][]string{{"Pinned", "Repositories", "Waiting"}, {"Pin", "Repos", "Wait"}}
	tests := []struct {
		name string
		maxW int
		want string
	}{
		{"titles", 100, "1 Pinned  [2] Repositories  3 Waiting"},
		{"short titles", 30, "1 Pin  [2] Repos  3 Wait"},
		{"keys alone", 10, "1 [2] 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, w := Foot(keys, tiers, 1, ui.Paint{}, ui.Paint{}, tt.maxW)
			if got != tt.want || w != ansi.StringWidth(tt.want) {
				t.Errorf("Foot = %q, %d; want %q", got, w, tt.want)
			}
		})
	}
}
