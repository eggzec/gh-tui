package issues

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

var sgrOpen = regexp.MustCompile("\x1b\\[([0-9;]+)m")

// boldRuns counts the styles a row starts that are bold and those that are
// not; the style that ends a run is not one.
func boldRuns(row string) (bold, plain int) {
	for _, m := range sgrOpen.FindAllStringSubmatch(row, -1) {
		switch {
		case m[1] == "0":
		case m[1] == "1" || strings.HasPrefix(m[1], "1;"):
			bold++
		default:
			plain++
		}
	}
	return bold, plain
}

// The selected row is bold in every cell and its title takes the accent, but
// only in a focused pane; a row that isn't selected is not bold.
func TestSelectedRowIsBold(t *testing.T) {
	t.Parallel()
	s := New(t.Context(), newFakeService(nil), config.Default().Keys)
	s.width = 120
	it := core.Issue{
		Number: 7, Title: "Title", State: core.StateOpen, Comments: 3, UpdatedAt: time.Now().Add(-time.Hour),
		Labels: []core.Label{{Name: "bug", Color: "d73a4a"}, {Name: "odd", Color: "zz"}},
	}
	it.Author.Login = "octo"
	pick := newPaint(s.theme.Pick).pre + "Title"

	s.focused = true
	row := s.renderRow(it, true, 120)
	if bold, plain := boldRuns(row); bold == 0 || plain != 0 {
		t.Errorf("focused selected row has %d bold and %d other styles, want every one bold: %q", bold, plain, row)
	}
	if !strings.Contains(row, pick) {
		t.Errorf("focused selected row = %q, want the title in the accent", row)
	}

	s.focused = false
	row = s.renderRow(it, true, 120)
	if bold, plain := boldRuns(row); bold == 0 || plain != 0 {
		t.Errorf("blurred selected row has %d bold and %d other styles, want every one bold: %q", bold, plain, row)
	}
	if strings.Contains(row, newPaint(s.theme.Pick).pre) {
		t.Errorf("blurred selected row = %q, want no accent", row)
	}

	for _, focused := range []bool{true, false} {
		s.focused = focused
		if bold, _ := boldRuns(s.renderRow(it, false, 120)); bold != 0 {
			t.Errorf("row that is not selected has %d bold styles, focused %v", bold, focused)
		}
	}
}
