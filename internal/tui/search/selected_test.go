package search

import (
	"regexp"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

var sgrOpen = regexp.MustCompile("\x1b\\[([0-9;]+)m")

// boldRuns counts the styles a result starts that are bold and those that
// are not; the style that ends a run is not one.
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

// The selected result is bold in every cell, all the lines of it, and its title
// or name takes the accent, but only in a focused pane; a result that isn't
// selected is not bold. The glyphs, such as that of a language, keep their
// color and are not made bold.
func TestSelectedHitIsBold(t *testing.T) {
	t.Parallel()
	s := newSection(t, flagged(), 120, 22)
	code := core.CodeHit{
		Repo: core.RepoRef{Owner: "octo", Name: "tea"}, Path: "tea.go",
		Fragments: []core.Fragment{{Text: "one tea\ntwo\nthree", Matches: [][2]int{{4, 7}}}},
	}
	cases := map[string]struct {
		render func(selected bool) string
		// title is what the accent starts in a focused pane.
		title string
		// matches says that the result has matches, which are bold in the
		// accent in any look.
		matches bool
	}{
		"repository": {render: func(sel bool) string { return s.renderHit(flagged().repos[3], sel, 120) }},
		"issue": {render: func(sel bool) string {
			return s.renderHit(issue(core.SearchIssues, "octo/tea", 12, "Terminal tea renders twice", core.StateOpen, false), sel, 120)
		}},
		"code": {render: func(sel bool) string { return s.renderCode(code, sel, 120) }, title: "tea.go", matches: true},
	}
	pick := newPaint(s.theme.Pick).pre
	for name, c := range cases {
		s.focused, s.area = true, resultsArea
		row := c.render(true)
		if bold, plain := boldRuns(row); bold == 0 || plain != 0 {
			t.Errorf("%s: focused selected row has %d bold and %d other styles, want every one bold: %q", name, bold, plain, row)
		}
		if !strings.Contains(row, pick+c.title) {
			t.Errorf("%s: focused selected row = %q, want the accent", name, row)
		}

		s.focused = false
		row = c.render(true)
		if bold, plain := boldRuns(row); bold == 0 || plain != 0 {
			t.Errorf("%s: blurred selected row has %d bold and %d other styles, want every one bold: %q", name, bold, plain, row)
		}
		if strings.Contains(row, pick+c.title) {
			t.Errorf("%s: blurred selected row = %q, want no accent", name, row)
		}

		if c.matches {
			continue
		}
		for _, focused := range []bool{true, false} {
			s.focused = focused
			if bold, _ := boldRuns(c.render(false)); bold != 0 {
				t.Errorf("%s: row that is not selected has %d bold styles, focused %v", name, bold, focused)
			}
		}
	}
}
