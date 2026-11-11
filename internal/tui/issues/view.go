package issues

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View implements ui.Section. It renders exactly the section's size.
func (s *Section) View() string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	if !s.hasRepo {
		return s.empty
	}
	if s.issuesOff() {
		return s.off
	}
	if s.height == 1 {
		return s.bar
	}
	if s.height == headsHeight+1 {
		return s.bar + "\n" + s.heads
	}
	return s.bar + "\n" + s.heads + "\n" + s.list.View()
}

// renderHeads names the columns of the rows, over the cells they take.
func (s *Section) renderHeads() {
	c := s.cols
	cols := []ui.Column{
		{Width: 2},
		{Title: "#", Width: ui.NumberWidth + 1},
		{Title: "Title", Width: c.title},
	}
	more := func(on bool, col ui.Column) {
		if on {
			cols = append(cols, ui.Column{Width: gap}, col)
		}
	}
	more(c.chips > 0, ui.Column{Title: "Labels", Width: c.labels})
	more(c.comments, ui.Column{Title: s.icons.Comment, Width: commentsWidth, Right: true})
	more(c.author, ui.Column{Title: "Author", Width: authorWidth})
	more(c.age, ui.Column{Title: ui.AgeTitle, Width: c.ageWidth})
	head := ui.Header(cols, 0, s.icons.Ellipsis)
	s.heads = strings.Repeat(" ", s.list.Gutter()) + s.theme.Muted.Render(termtext.Fit(head, max(s.width-s.list.Gutter(), 0), ""))
}

// renderChrome renders what surrounds the bubbles: the bar over the list,
// with the count and the tabs, and the empty state.
func (s *Section) renderChrome() {
	// The rows get the width less what the list draws before them.
	w := max(s.width-s.list.Gutter(), 0)
	s.cols, s.colsWidth = layout(w, s.room, s.ageRoom()), w
	s.renderBar()
	s.renderHeads()
	s.renderEmpty()
	s.renderOff()
}

func (s *Section) renderBar() {
	t := s.theme
	left := strings.Repeat(" ", s.list.Gutter())
	var right strings.Builder
	for i, tb := range tabs {
		if i > 0 {
			right.WriteString(t.Subtle.Render(s.icons.Separator))
		}
		st := t.Subtle
		if tb.state == s.tab {
			st = t.Accent
		}
		right.WriteString(st.Render(tb.label))
	}
	// Narrow panes name only the tab shown.
	lw, tabsRight := ansi.StringWidth(left), right.String()
	if lw+ansi.StringWidth(tabsRight)+2 > s.width {
		tabsRight = t.Accent.Render(tabLabel(s.tab))
	}
	s.bar = spread(left, tabsRight, s.width, s.icons.Ellipsis)
}

// CountForms is the forms of the count of the issues for the pane's frame,
// longest first (see ui.CountForms). There is none while the list loads. The
// total counts what a search of the list's text finds, which for a milestone
// filter may differ from the list.
func (s *Section) CountForms() []string {
	f := &s.list
	if s.total == nil || f.Len() == 0 && (!f.Done() || f.Err() != nil) {
		return nil
	}
	// The feed shows the pinned issues above and counts none of them, yet
	// the total and the pages hold them. They match the list, so they count
	// as matched too unless a quick filter may hide them.
	pins := 0
	if s.pins != nil {
		pins = s.pins.dropped()
	}
	quick := f.FilterQuery() != ""
	matched := f.Shown()
	if !quick {
		matched += pins
	}
	return ui.CountForms(ui.CountSpec{
		Total:    int(s.total.Load()),
		Loaded:   f.Loaded() + pins,
		Matched:  matched,
		More:     !f.Done(),
		Filtered: s.query != "" || quick,
	})
}

// spread puts left and right at the two ends of a line width cells wide,
// cutting left to end in tail if both don't fit.
func spread(left, right string, width int, tail string) string {
	rw := ansi.StringWidth(right)
	if rw+1 > width {
		return termtext.Fit(left, width, tail)
	}
	left = termtext.Truncate(left, width-rw-1, tail)
	return left + strings.Repeat(" ", width-ansi.StringWidth(left)-rw) + right
}

// renderEmpty renders the state shown before a repository is selected,
// wrapped to fit a narrow pane.
func (s *Section) renderEmpty() {
	s.empty = s.placard("No repository selected", s.hint)
}

// renderOff renders the state shown in place of the list when the
// repository turned its issues off, which only an admin can undo.
func (s *Section) renderOff() {
	if !s.issuesOff() {
		s.off = ""
		return
	}
	hint := "The repository doesn't use GitHub's issues."
	if s.caps.Permission == core.PermissionAdmin {
		hint = "You can turn them on in its settings on GitHub."
	}
	s.off = s.placard("Issues are turned off for "+s.repo.String(), hint)
}

// placard renders title over hint in the middle of the section, wrapped
// to fit a narrow pane.
func (s *Section) placard(title, hint string) string {
	if s.width <= 0 || s.height <= 0 {
		return ""
	}
	t := s.theme
	center := lipgloss.NewStyle().Width(s.width).Align(lipgloss.Center)
	text := lipgloss.JoinVertical(lipgloss.Left,
		center.Inherit(t.Title).Render(title),
		"",
		center.Inherit(t.Muted).Render(hint),
	)
	lines := strings.Split(lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, text), "\n")
	lines = lines[:min(len(lines), s.height)]
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, s.width, "")
	}
	return strings.Join(lines, "\n")
}

// emptyText is what the list says when no issue is in the tab, with the
// key that shows more.
func (s *Section) emptyText() string {
	kind := strings.ToLower(tabLabel(s.tab)) + " issues"
	if s.tab == core.FilterAll {
		kind = "issues"
	}
	if s.query != "" {
		return ui.NoMatch(kind, ui.KeyOf(s.icons, s.keys.ClearFilter))
	}
	text := ui.None(kind)
	switch next := nextTab(s.tab, 1); {
	case s.tab == core.FilterAll:
		return text
	case next == core.FilterAll:
		return ui.Press(text, ui.KeyOf(s.icons, s.keys.NextTab), "show all of them")
	default:
		return ui.Press(text, ui.KeyOf(s.icons, s.keys.NextTab), "show "+strings.ToLower(tabLabel(next))+" ones")
	}
}
