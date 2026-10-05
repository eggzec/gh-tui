package ownerui

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestCardWidth(t *testing.T) {
	tests := []struct {
		name         string
		width, cards int
		// first and last are the widths of the first and the last card of
		// the first row.
		first, last int
	}{
		// 116 cells hold four cards of at least MinCardWidth, and the last
		// takes the cells left over.
		{"a full row", 116, 4, 27, 29},
		{"exactly a row", 110, 4, 26, 26},
		{"two rows", 116, 5, 27, 29},
		{"more cards than a row holds", 116, 9, 27, 29},
		{"two cards share the row", 116, 2, 57, 57},
		{"three cards share it", 116, 3, 37, 38},
		{"one card is capped", 116, 1, MaxCardWidth, MaxCardWidth},
		{"few cards on a wide pane are capped", 188, 2, MaxCardWidth, MaxCardWidth},
		{"a narrow pane", 20, 3, 20, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Cards
			c.Set(make([]core.Repo, tt.cards))
			c.Resize(tt.width, CardHeight)
			last := min(c.Cols, tt.cards) - 1
			if got := c.CardWidth(tt.width, 0); got != tt.first {
				t.Errorf("the first card is %d wide, want %d", got, tt.first)
			}
			if got := c.CardWidth(tt.width, last); got != tt.last {
				t.Errorf("the last card is %d wide, want %d", got, tt.last)
			}
		})
	}
}
