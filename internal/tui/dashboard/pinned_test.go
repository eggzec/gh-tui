package dashboard

import (
	"strings"
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
		// 116 cells hold four cards of at least minCardWidth, and the last
		// takes the cells left over.
		{"a full row", 116, 4, 27, 29},
		{"exactly a row", 110, 4, 26, 26},
		{"two rows", 116, 5, 27, 29},
		{"more cards than a row holds", 116, 9, 27, 29},
		{"two cards share the row", 116, 2, 57, 57},
		{"three cards share it", 116, 3, 37, 38},
		{"one card is capped", 116, 1, maxCardWidth, maxCardWidth},
		{"few cards on a wide pane are capped", 188, 2, maxCardWidth, maxCardWidth},
		{"a narrow pane", 20, 3, 20, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c cards
			c.set(make([]core.Repo, tt.cards))
			c.resize(tt.width, pinnedHeight-2)
			last := min(c.cols, tt.cards) - 1
			if got := c.cardWidth(tt.width, 0); got != tt.first {
				t.Errorf("the first card is %d wide, want %d", got, tt.first)
			}
			if got := c.cardWidth(tt.width, last); got != tt.last {
				t.Errorf("the last card is %d wide, want %d", got, tt.last)
			}
		})
	}
}

func TestOnePinUsesThePane(t *testing.T) {
	svc := newFake()
	desc := "A GitHub client for the terminal, built with Bubble Tea"
	svc.header.Pinned = []core.Repo{{Ref: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}, Description: desc, Language: "Go"}}
	s := newSection(t, svc, &fakeInbox{}, 120, 36, WithHere(core.RepoRef{}, nil))
	if !strings.Contains(screen(s), desc) {
		t.Errorf("the description of the only pin doesn't show on one line:\n%s", screen(s))
	}
}
