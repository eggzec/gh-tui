package pulls

import (
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestStateGlyphs(t *testing.T) {
	h := started(t, newFakeService(), 100, 10, WithIcons(ui.NewIcons(config.IconsASCII)))
	s := h.Section
	for _, tt := range []struct {
		state core.State
		draft bool
		row   string
		badge string
	}{
		{core.StateOpen, false, "O #1 ", " O Open "},
		{core.StateOpen, true, "D #1 ", " D Draft "},
		{core.StateMerged, false, "M #1 ", " M Merged "},
		{core.StateClosed, false, "X #1 ", " X Closed "},
		// A closed draft was declined all the same.
		{core.StateClosed, true, "X #1 ", " X Closed "},
	} {
		pr := core.PullRequest{Draft: tt.draft}
		pr.Number, pr.Title, pr.State = 1, "T", tt.state
		if row := ansi.Strip(s.renderRow(pr, false, 100)); !strings.HasPrefix(row, tt.row) {
			t.Errorf("%s draft %v: row = %q, want it to start with %q", tt.state, tt.draft, row, tt.row)
		}
		if badge := ansi.Strip(s.st.badge(pr)); badge != tt.badge {
			t.Errorf("%s draft %v: badge = %q, want %q", tt.state, tt.draft, badge, tt.badge)
		}
	}
}

// Numbers too long for their column take the extra cells from the title,
// so the row keeps its width and the space after the number. A row too
// narrow for the glyph and the number is cut.
func TestLongNumbers(t *testing.T) {
	h := started(t, newFakeService(), 100, 10, WithIcons(ui.NewIcons(config.IconsASCII)))
	s := h.Section
	for _, tt := range []struct {
		number int
		row    string
	}{
		{7, "O #7     "},
		{12345, "O #12345 "},
		{123456, "O #123456 "},
		{1234567890, "O #1234567890 "},
	} {
		pr := core.PullRequest{}
		pr.Number, pr.Title, pr.State = tt.number, "Title", core.StateOpen
		for _, width := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 20, 40, 80, 120} {
			row := ansi.Strip(s.renderRow(pr, false, width))
			if want := tt.row[:min(len(tt.row), width)]; !strings.HasPrefix(row, want) {
				t.Errorf("#%d at %d: row = %q, want it to start with %q", tt.number, width, row, want)
			}
			if w := ansi.StringWidth(row); w != width {
				t.Errorf("#%d at %d: row is %d cells wide", tt.number, width, w)
			}
		}
	}
}

// The review and checks columns take the glyphs of the icon set, the
// checks those of the checks screen, so the ASCII set draws them in ASCII.
func TestReviewAndChecksGlyphs(t *testing.T) {
	h := started(t, newFakeService(), 100, 10, WithIcons(ui.NewIcons(config.IconsASCII)))
	s := h.Section
	for _, tt := range []struct {
		review core.ReviewDecision
		checks core.ChecksState
		want   string
	}{
		{core.ReviewApproved, core.ChecksSuccess, "  + +  "},
		{core.ReviewChangesRequested, core.ChecksFailure, "  ~ x  "},
		{core.ReviewRequired, core.ChecksPending, "  ? .  "},
	} {
		pr := core.PullRequest{ReviewDecision: tt.review, Checks: tt.checks}
		pr.Number, pr.Title, pr.State = 1, "T", core.StateOpen
		row := ansi.Strip(s.renderRow(pr, false, 100))
		if !strings.Contains(row, tt.want) {
			t.Errorf("%s, %s: row = %q, want %q in it", tt.review, tt.checks, row, tt.want)
		}
	}
}

// With the ASCII icons a row is ASCII: its signs, the more after the
// label and the ellipses of cut text.
func TestRowASCII(t *testing.T) {
	h := started(t, newFakeService(), 140, 10, WithIcons(ui.NewIcons(config.IconsASCII)))
	s := h.Section
	labels := make([]core.Label, 12)
	for i := range labels {
		labels[i] = core.Label{Name: "a-long-label-name"}
	}
	pr := core.PullRequest{Labels: labels, Additions: 120, Deletions: 40}
	pr.Number, pr.Title, pr.State = 1, strings.Repeat("A long title ", 10), core.StateOpen
	pr.Author.Login = "a-very-long-login"
	row := ansi.Strip(s.renderRow(pr, false, 140))
	if strings.ContainsFunc(row, func(r rune) bool { return r > unicode.MaxASCII }) {
		t.Errorf("row %q isn't ASCII", row)
	}
	if !strings.Contains(row, "...") || !strings.Contains(row, "-40") {
		t.Errorf("row %q lacks a cut or the deletions", row)
	}
}
