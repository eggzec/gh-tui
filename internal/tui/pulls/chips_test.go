package pulls

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// A row draws the first label as a chip in the label's color, with the count
// of the others, in the width of the labels column.
func TestRowDrawsLabelChip(t *testing.T) {
	t.Parallel()
	h := started(t, newFakeService(), 120, 10)
	s := h.Section
	pr := core.PullRequest{}
	pr.Number, pr.Title, pr.State = 7, "Title", core.StateOpen
	pr.Labels = []core.Label{{Name: "a-long-label-name", Color: "d73a4a"}, {Name: "ui"}}

	row := s.renderRow(pr, false, 120)
	chip, w := s.st.chips.Get(pr.Labels[0])
	if !strings.Contains(row, chip) {
		t.Fatalf("row = %q, want the chip %q", row, chip)
	}
	if !strings.Contains(chip, "\x1b[") || !strings.Contains(chip, "48;") {
		t.Errorf("chip = %q, want a colored background", chip)
	}
	if got := w + ansi.StringWidth(" +1"); got > labelsWidth {
		t.Errorf("chip and count take %d cells, want at most %d", got, labelsWidth)
	}
	if got := lipgloss.Width(row); got != 120 {
		t.Errorf("row is %d cells wide, want 120", got)
	}
}
