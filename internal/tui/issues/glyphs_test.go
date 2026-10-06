package issues

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
	s := New(t.Context(), newFakeService(nil), config.Default().Keys, WithIcons(ui.NewIcons(config.IconsASCII)))
	for _, tt := range []struct {
		state  core.State
		reason core.StateReason
		row    string
		badge  string
	}{
		{core.StateOpen, "", "o #1     T", "o Open"},
		{core.StateOpen, core.ReasonReopened, "o #1     T", "o Open"},
		{core.StateClosed, "", "x #1     T", "x Closed"},
		{core.StateClosed, core.ReasonCompleted, "x #1     T", "x Closed"},
		{core.StateClosed, core.ReasonNotPlanned, "- #1     T", "- Closed as not planned"},
		{core.StateClosed, core.ReasonDuplicate, "- #1     T", "- Closed as not planned"},
	} {
		it := core.Issue{Number: 1, Title: "T", State: tt.state, Reason: tt.reason}
		if row := ansi.Strip(s.renderRow(it, false, 80)); !strings.HasPrefix(row, tt.row) {
			t.Errorf("%s %s: row = %q, want it to start with %q", tt.state, tt.reason, row, tt.row)
		}
		if badge := ansi.Strip(s.rows.badges[ui.IssueState(it)]); badge != tt.badge {
			t.Errorf("%s %s: badge = %q, want %q", tt.state, tt.reason, badge, tt.badge)
		}
	}
}

// Numbers too long for their column take the extra cells from the title,
// so the row keeps its width and the space after the number. A row too
// narrow for the glyph and the number is cut.
func TestLongNumbers(t *testing.T) {
	s := New(t.Context(), newFakeService(nil), config.Default().Keys, WithIcons(ui.NewIcons(config.IconsASCII)))
	for _, tt := range []struct {
		number int
		row    string
	}{
		{7, "o #7     "},
		{12345, "o #12345 "},
		{123456, "o #123456 "},
		{1234567890, "o #1234567890 "},
	} {
		it := core.Issue{Number: tt.number, Title: "Title", State: core.StateOpen}
		for _, width := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 20, 40, 80, 120} {
			row := ansi.Strip(s.renderRow(it, false, width))
			if want := tt.row[:min(len(tt.row), width)]; !strings.HasPrefix(row, want) {
				t.Errorf("#%d at %d: row = %q, want it to start with %q", tt.number, width, row, want)
			}
			if w := ansi.StringWidth(row); w != width {
				t.Errorf("#%d at %d: row is %d cells wide", tt.number, width, w)
			}
		}
	}
}

// With the ASCII icons a row is ASCII: its comment mark, the more after
// the labels and the ellipses of cut text.
func TestRowASCII(t *testing.T) {
	s := New(t.Context(), newFakeService(nil), config.Default().Keys, WithIcons(ui.NewIcons(config.IconsASCII)))
	labels := make([]core.Label, 12)
	for i := range labels {
		labels[i] = core.Label{Name: "a-long-label-name", Color: "d73a4a"}
	}
	it := core.Issue{
		Number: 1, Title: strings.Repeat("A long title ", 10), State: core.StateOpen, Comments: 12,
		Labels: labels, Author: core.User{Login: "a-very-long-login"},
	}
	s.room = labelRoom{labelsCap(1), labelsCap(2)}
	for _, width := range []int{60, 120} {
		row := ansi.Strip(s.renderRow(it, false, width))
		if strings.ContainsFunc(row, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("width %d: row %q isn't ASCII", width, row)
		}
		if !strings.Contains(row, "...") {
			t.Errorf("width %d: row %q cuts nothing with ...", width, row)
		}
	}
}

// With the ASCII icons the detail is ASCII apart from what GitHub wrote:
// its header, the separators and marks of its comments, and its frame.
func TestDetailASCII(t *testing.T) {
	svc := newFakeService(sampleIssues(3))
	svc.addComments(999, sampleComments(3)...)
	h := started(t, svc, 80, 30, WithIcons(ui.NewIcons(config.IconsASCII)))
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	v := ansi.Strip(m.header(m.issue) + "\n" + m.View())
	if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
		t.Errorf("detail isn't ASCII:\n%s", v)
	}
}
